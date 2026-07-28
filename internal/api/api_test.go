package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	s := New(cfg, nil)
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
