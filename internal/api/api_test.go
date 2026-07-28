package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"saveguardian/internal/config"
)

func testServer(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Port = 8787 // matches the default; make the allowed origin explicit
	s := New(cfg, 8787, nil, nil)
	mux := http.NewServeMux()
	s.Register(mux)
	return s, mux
}

// TestGuardOriginPolicy verifies the CSRF guard: same-origin and empty-origin
// requests pass; foreign origins and wrong ports are rejected with 403.
func TestGuardOriginPolicy(t *testing.T) {
	_, mux := testServer(t)
	cases := []struct {
		origin string
		want   int
	}{
		{"", http.StatusOK},                      // non-browser / same-origin GET
		{"http://127.0.0.1:8787", http.StatusOK}, // the web UI itself
		{"http://localhost:8787", http.StatusOK}, // localhost alias
		{"http://evil.example.com", http.StatusForbidden},
		{"http://127.0.0.1:9999", http.StatusForbidden}, // right host, wrong port
		{"null", http.StatusForbidden},                  // sandboxed/file origin
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("origin %q: got status %d, want %d", c.origin, rec.Code, c.want)
		}
	}
}

// TestConfigEndpointReturnsGames confirms an allowed request reaches the handler
// and serializes the config (which also goes through Config.MarshalJSON).
func TestConfigEndpointReturnsGames(t *testing.T) {
	s, mux := testServer(t)
	if err := s.cfg.UpsertGame(config.Game{Name: "G", Source: "/s", BackupRoot: "/b"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"name":"G"`) {
		t.Fatalf("config body missing game: %s", body)
	}
}

// TestConcurrentBackupsAreSerialized fires many backups at once. Without the
// server op-lock, concurrent Create calls in the same clock-second would race
// on timestamp-directory selection and collide; with it, each backup lands in
// its own directory. Run under -race for full effect.
func TestConcurrentBackupsAreSerialized(t *testing.T) {
	s, mux := testServer(t)
	root := t.TempDir()
	src := filepath.Join(root, "save")
	bak := filepath.Join(root, "bak")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.sav"), []byte("DATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.cfg.UpsertGame(config.Game{Name: "G", Source: src, BackupRoot: bak}); err != nil {
		t.Fatal(err)
	}

	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/backup",
				strings.NewReader(`{"game":"G","note":"","type":"manual"}`))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("backup failed: %d %s", rec.Code, rec.Body.String())
			}
		}()
	}
	wg.Wait()

	m, err := s.managerFor("G")
	if err != nil {
		t.Fatal(err)
	}
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != n {
		t.Fatalf("expected %d distinct backups, got %d", n, len(list))
	}
}

func TestVerifyEndpoint(t *testing.T) {
	s, mux := testServer(t)
	root := t.TempDir()
	src := filepath.Join(root, "save")
	bak := filepath.Join(root, "bak")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.sav"), []byte("DATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.cfg.UpsertGame(config.Game{Name: "G", Source: src, BackupRoot: bak}); err != nil {
		t.Fatal(err)
	}
	m, _ := s.managerFor("G")
	meta, err := m.Create("", "manual")
	if err != nil {
		t.Fatal(err)
	}

	get := func() string {
		req := httptest.NewRequest(http.MethodGet,
			"/api/verify?game=G&timestamp="+meta.Timestamp, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("verify status %d: %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	if body := get(); !strings.Contains(body, `"ok":true`) {
		t.Fatalf("expected intact backup, got %s", body)
	}
	// Corrupt a backed-up file and re-check.
	if err := os.WriteFile(filepath.Join(bak, meta.Timestamp, "a.sav"), []byte("X"), 0o644); err != nil {
		t.Fatal(err)
	}
	if body := get(); !strings.Contains(body, `"ok":false`) || !strings.Contains(body, "a.sav") {
		t.Fatalf("expected corruption reported, got %s", body)
	}
}

func TestHotkeyEndpoint(t *testing.T) {
	s, mux := testServer(t)
	post := func(hk string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/hotkey",
			strings.NewReader(`{"hotkey":`+`"`+hk+`"}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	// A valid hotkey is accepted and persisted.
	if rec := post("Ctrl+Alt+F5"); rec.Code != http.StatusOK {
		t.Fatalf("valid hotkey rejected: %d %s", rec.Code, rec.Body.String())
	}
	if s.cfg.Hotkey != "Ctrl+Alt+F5" {
		t.Fatalf("hotkey not persisted: %q", s.cfg.Hotkey)
	}
	// An invalid hotkey (no main key) is rejected and does not change config.
	if rec := post("Ctrl+Alt"); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid hotkey accepted: %d", rec.Code)
	}
	if s.cfg.Hotkey != "Ctrl+Alt+F5" {
		t.Fatalf("config changed on invalid hotkey: %q", s.cfg.Hotkey)
	}
}

func TestPortEndpoint(t *testing.T) {
	s, mux := testServer(t)
	post := func(p string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/port", strings.NewReader(`{"port":`+p+`}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("9001"); rec.Code != http.StatusOK {
		t.Fatalf("valid port rejected: %d %s", rec.Code, rec.Body.String())
	}
	if s.cfg.Port != 9001 {
		t.Fatalf("port not persisted: %d", s.cfg.Port)
	}
	// The same-origin check must still use the actually-bound port (8787),
	// not the changed desired port, so the running UI keeps working.
	if !s.allowedOrigin("http://127.0.0.1:8787") || s.allowedOrigin("http://127.0.0.1:9001") {
		t.Fatal("guard should track the bound port, not the changed cfg.Port")
	}
	if rec := post("70000"); rec.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range port accepted: %d", rec.Code)
	}
}
