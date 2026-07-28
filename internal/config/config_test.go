package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLoadCreatesDefault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Hotkey != DefaultHotkey || c.Port != DefaultPort {
		t.Fatalf("defaults not applied: %+v", c)
	}
	// File should have been written.
	if _, err := Load(p); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
}

func TestUpsertAndDeleteGame(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, _ := Load(p)
	g := Game{Name: "Plague", Source: "/src", BackupRoot: "/bak"}
	if err := c.UpsertGame(g); err != nil {
		t.Fatal(err)
	}
	if c.ActiveGame != "Plague" {
		t.Fatalf("active not set on first game: %q", c.ActiveGame)
	}
	// Persisted across reload.
	c2, _ := Load(p)
	if _, ok := c2.Game("Plague"); !ok {
		t.Fatal("game not persisted")
	}
	// Update existing (no duplicate).
	g.BackupRoot = "/bak2"
	c2.UpsertGame(g)
	if len(c2.Games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(c2.Games))
	}
	got, _ := c2.Game("Plague")
	if got.BackupRoot != "/bak2" {
		t.Fatalf("update failed: %+v", got)
	}
	// Delete.
	if err := c2.DeleteGame("Plague"); err != nil {
		t.Fatal(err)
	}
	if len(c2.Games) != 0 || c2.ActiveGame != "" {
		t.Fatalf("delete did not clear state: %+v", c2)
	}
}

func TestUpsertValidation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, _ := Load(p)
	if err := c.UpsertGame(Game{Name: "", Source: "/s", BackupRoot: "/b"}); err == nil {
		t.Fatal("expected error for empty name")
	}
	if err := c.UpsertGame(Game{Name: "x", Source: "", BackupRoot: "/b"}); err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestLegacySourceRootMigration(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	// Write a legacy config using the old "sourceRoot" key.
	legacy := `{"activeGame":"Old","hotkey":"Ctrl+Alt+S","port":8787,` +
		`"games":[{"name":"Old","sourceRoot":"/legacy/src","backupRoot":"/bak"}]}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	g, ok := c.Game("Old")
	if !ok || g.Source != "/legacy/src" {
		t.Fatalf("legacy sourceRoot not migrated: %+v", g)
	}
	if g.LegacySourceRoot != "" {
		t.Fatalf("legacy field not cleared: %+v", g)
	}
}

// TestConcurrentMarshalAndUpsert exercises the config lock: marshaling (the
// /api/config response path) must not race with concurrent game mutations.
// Meaningful under `go test -race`.
func TestConcurrentMarshalAndUpsert(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, _ := Load(p)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			_ = c.UpsertGame(Game{Name: fmt.Sprintf("g%d", n), Source: "/s", BackupRoot: "/b"})
		}(i)
		go func() {
			defer wg.Done()
			if _, err := json.Marshal(c); err != nil {
				t.Errorf("marshal failed: %v", err)
			}
		}()
	}
	wg.Wait()
}
