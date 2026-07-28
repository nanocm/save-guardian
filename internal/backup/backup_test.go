package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixedNow() func() time.Time {
	t := time.Date(2026, 7, 22, 15, 30, 12, 0, time.UTC)
	return func() time.Time {
		cur := t
		t = t.Add(time.Second)
		return cur
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// setup creates a source folder (the backup unit) and a separate backup root.
func setup(t *testing.T) (src, bak string) {
	t.Helper()
	root := t.TempDir()
	src = filepath.Join(root, "maingame1")
	bak = filepath.Join(root, "Backups")
	writeFile(t, filepath.Join(src, "maingame1.sav"), "SAVE-DATA-V1")
	writeFile(t, filepath.Join(src, "sub", "extra.dat"), "EXTRA")
	return src, bak
}

func TestCreateBackupCopiesEntireSourceFolder(t *testing.T) {
	src, bak := setup(t)
	m, err := New("g", src, bak, fixedNow())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := m.Create("before boss", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Note != "before boss" || meta.Type != "manual" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if len(meta.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(meta.Files))
	}
	if got := readFile(t, filepath.Join(bak, meta.Timestamp, "maingame1.sav")); got != "SAVE-DATA-V1" {
		t.Fatalf("backup content mismatch: %q", got)
	}
	if got := readFile(t, filepath.Join(bak, meta.Timestamp, "sub", "extra.dat")); got != "EXTRA" {
		t.Fatalf("nested file not copied: %q", got)
	}
	if _, err := os.Stat(filepath.Join(bak, meta.Timestamp, metaFileName)); err != nil {
		t.Fatalf("meta.json missing: %v", err)
	}
}

func TestChecksumsRecorded(t *testing.T) {
	src, bak := setup(t)
	m, _ := New("g", src, bak, fixedNow())
	meta, err := m.Create("", "manual")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range meta.Files {
		if len(f.SHA256) != 64 {
			t.Fatalf("bad sha256 for %s: %q", f.Path, f.SHA256)
		}
		if f.Size == 0 {
			t.Fatalf("zero size for %s", f.Path)
		}
	}
}

func TestListNewestFirst(t *testing.T) {
	src, bak := setup(t)
	m, _ := New("g", src, bak, fixedNow())
	m.Create("one", "manual")
	m.Create("two", "manual")
	metas, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(metas))
	}
	if metas[0].Timestamp < metas[1].Timestamp {
		t.Fatalf("not sorted newest-first: %v", metas)
	}
}

func TestRestoreOverwritesSourceAndMakesSafetySnapshot(t *testing.T) {
	src, bak := setup(t)
	m, _ := New("g", src, bak, fixedNow())
	orig, err := m.Create("checkpoint", "manual")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the game overwriting the save with a death penalty.
	savePath := filepath.Join(src, "maingame1.sav")
	writeFile(t, savePath, "SAVE-DATA-DEAD")

	safety, err := m.Restore(orig.Timestamp)
	if err != nil {
		t.Fatal(err)
	}
	if safety == nil || safety.Type != "pre-restore" {
		t.Fatalf("expected pre-restore safety snapshot, got %+v", safety)
	}
	if got := readFile(t, savePath); got != "SAVE-DATA-V1" {
		t.Fatalf("restore did not overwrite source: %q", got)
	}
	deadCopy := readFile(t, filepath.Join(bak, safety.Timestamp, "maingame1.sav"))
	if deadCopy != "SAVE-DATA-DEAD" {
		t.Fatalf("safety snapshot content wrong: %q", deadCopy)
	}
	if _, err := os.Stat(filepath.Join(src, metaFileName)); !os.IsNotExist(err) {
		t.Fatalf("meta.json leaked into source folder")
	}
}

func TestDeleteBackup(t *testing.T) {
	src, bak := setup(t)
	m, _ := New("g", src, bak, fixedNow())
	meta, _ := m.Create("", "manual")
	if err := m.Delete(meta.Timestamp); err != nil {
		t.Fatal(err)
	}
	metas, _ := m.List()
	if len(metas) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(metas))
	}
}

func TestUpdateNote(t *testing.T) {
	src, bak := setup(t)
	m, _ := New("g", src, bak, fixedNow())
	meta, _ := m.Create("old", "manual")
	if err := m.UpdateNote(meta.Timestamp, "new note"); err != nil {
		t.Fatal(err)
	}
	metas, _ := m.List()
	if metas[0].Note != "new note" {
		t.Fatalf("note not updated: %q", metas[0].Note)
	}
}

func TestNestedRootsRejected(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "save")
	bak := filepath.Join(src, "Backups") // nested inside src
	os.MkdirAll(bak, 0o755)
	if _, err := New("g", src, bak, nil); err == nil {
		t.Fatal("expected error for nested roots")
	}
}

func TestTimestampTraversalRejected(t *testing.T) {
	src, bak := setup(t)
	m, _ := New("g", src, bak, fixedNow())
	if err := m.Delete("../../etc"); err == nil {
		t.Fatal("expected traversal rejection on delete")
	}
	if _, err := m.Restore("../evil"); err == nil {
		t.Fatal("expected traversal rejection on restore")
	}
}

func TestVerifyDetectsCorruption(t *testing.T) {
	src, bak := setup(t)
	m, _ := New("g", src, bak, fixedNow())
	meta, err := m.Create("v", "manual")
	if err != nil {
		t.Fatal(err)
	}
	// An intact backup verifies clean.
	if bad, err := m.Verify(meta.Timestamp); err != nil || len(bad) != 0 {
		t.Fatalf("expected clean verify, got bad=%v err=%v", bad, err)
	}
	// Tampering a backed-up file is detected.
	writeFile(t, filepath.Join(bak, meta.Timestamp, "maingame1.sav"), "TAMPERED")
	bad, err := m.Verify(meta.Timestamp)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 1 || bad[0] != "maingame1.sav" {
		t.Fatalf("expected maingame1.sav flagged as corrupt, got %v", bad)
	}
}

func TestRestoreRejectsCorruptBackup(t *testing.T) {
	src, bak := setup(t)
	m, _ := New("g", src, bak, fixedNow())
	meta, err := m.Create("cp", "manual")
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt the backup, then change the live save to a distinct value.
	writeFile(t, filepath.Join(bak, meta.Timestamp, "maingame1.sav"), "CORRUPT")
	live := filepath.Join(src, "maingame1.sav")
	writeFile(t, live, "LIVE-STATE")

	safety, err := m.Restore(meta.Timestamp)
	if err == nil {
		t.Fatal("expected restore to reject a corrupt backup")
	}
	if safety != nil {
		t.Fatalf("no pre-restore snapshot should be created for a corrupt backup, got %+v", safety)
	}
	if got := readFile(t, live); got != "LIVE-STATE" {
		t.Fatalf("live save must be left untouched, got %q", got)
	}
}
