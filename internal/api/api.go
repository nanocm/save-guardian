package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"saveguardian/internal/backup"
	"saveguardian/internal/config"
)

// FolderPicker opens a native folder chooser and returns the selected path.
type FolderPicker func() (string, error)

// hub broadcasts short event strings to all connected SSE clients so the web
// UI refreshes instantly after any backup/restore, including hotkey backups.
type hub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func newHub() *hub { return &hub{subs: make(map[chan string]struct{})} }

func (h *hub) subscribe() chan string {
	ch := make(chan string, 8)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan string) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *hub) broadcast(msg string) {
	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
	h.mu.Unlock()
}

// Server wires the HTTP API to config and backup logic.
type Server struct {
	cfg  *config.Config
	pick FolderPicker
	hub  *hub
}

// New creates an API server.
func New(cfg *config.Config, pick FolderPicker) *Server {
	if pick == nil {
		pick = func() (string, error) { return "", nil }
	}
	return &Server{cfg: cfg, pick: pick, hub: newHub()}
}

// Register attaches API routes to the given mux. Every /api handler is wrapped
// with guard so that cross-origin requests (a malicious web page trying to
// trigger a destructive restore/delete against the local server) are rejected.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/config", s.guard(s.handleConfig))
	mux.HandleFunc("/api/games", s.guard(s.handleGames))
	mux.HandleFunc("/api/active", s.guard(s.handleActive))
	mux.HandleFunc("/api/backups", s.guard(s.handleBackups))
	mux.HandleFunc("/api/backups/batch-delete", s.guard(s.handleBatchDelete))
	mux.HandleFunc("/api/backup", s.guard(s.handleBackup))
	mux.HandleFunc("/api/restore", s.guard(s.handleRestore))
	mux.HandleFunc("/api/pick-folder", s.guard(s.handlePickFolder))
	mux.HandleFunc("/api/events", s.guard(s.handleEvents))
}

// guard rejects requests whose Origin header is present but does not match the
// local server. Browsers attach Origin to cross-site fetches (including
// "simple" text/plain POSTs that skip the CORS preflight), so this blocks CSRF
// against state-changing endpoints while leaving the same-origin web UI and
// non-browser clients (empty Origin) working.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && !s.allowedOrigin(origin) {
			writeErr(w, http.StatusForbidden, "cross-origin request rejected")
			return
		}
		next(w, r)
	}
}

// allowedOrigin reports whether origin is this local server. Port is set once
// at load and never mutated, so reading it here is race-free.
func (s *Server) allowedOrigin(origin string) bool {
	return origin == fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Port) ||
		origin == fmt.Sprintf("http://localhost:%d", s.cfg.Port)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// managerFor builds a backup.Manager for the named game.
func (s *Server) managerFor(name string) (*backup.Manager, error) {
	g, ok := s.cfg.Game(name)
	if !ok {
		return nil, errNotFound("game not found: " + name)
	}
	return backup.New(g.Name, g.Source, g.BackupRoot, nil)
}

type notFoundError string

func (e notFoundError) Error() string { return string(e) }
func errNotFound(msg string) error    { return notFoundError(msg) }

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg)
}

func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var g config.Game
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.cfg.UpsertGame(g); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.cfg)
	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if err := s.cfg.DeleteGame(name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.cfg)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleActive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.SetActive(body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.cfg)
}

func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request) {
	game := r.URL.Query().Get("game")
	m, err := s.managerFor(game)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.Method == http.MethodDelete {
		if err := m.Delete(r.URL.Query().Get("timestamp")); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.hub.broadcast("update")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if r.Method == http.MethodPatch {
		var body struct {
			Timestamp string `json:"timestamp"`
			Note      string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := m.UpdateNote(body.Timestamp, body.Note); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.hub.broadcast("update")
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	list, err := m.List()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": list})
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Game string `json:"game"`
		Note string `json:"note"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := s.managerFor(body.Game)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	meta, err := m.Create(body.Note, body.Type)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.hub.broadcast("update")
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Game      string `json:"game"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := s.managerFor(body.Game)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	safety, err := m.Restore(body.Timestamp)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.broadcast("update")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "safetySnapshot": safety})
}

func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	path, err := s.pick()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// handleBatchDelete deletes multiple backups in one request and broadcasts a
// single UI update. Timestamps that fail to delete are returned in "failed".
func (s *Server) handleBatchDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Game       string   `json:"game"`
		Timestamps []string `json:"timestamps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := s.managerFor(body.Game)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	failed := []string{}
	for _, ts := range body.Timestamps {
		if err := m.Delete(ts); err != nil {
			failed = append(failed, ts)
		}
	}
	s.hub.broadcast("update")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "failed": failed})
}

// handleEvents streams server-sent events so the UI updates in real time.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// HotkeyBackup backs up the active game's source folder and notifies the UI.
// It returns the created backup meta so callers can give audible feedback.
func (s *Server) HotkeyBackup() (*backup.Meta, error) {
	m, err := s.managerFor(s.cfg.Active())
	if err != nil {
		return nil, err
	}
	meta, err := m.Create("热键备份", "hotkey")
	if err != nil {
		return nil, err
	}
	s.hub.broadcast("hotkey")
	return meta, nil
}
