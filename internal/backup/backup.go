package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileInfo records a single file's integrity metadata within a backup.
type FileInfo struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Meta is the metadata stored alongside each backup.
type Meta struct {
	Note      string     `json:"note"`
	CreatedAt time.Time  `json:"createdAt"`
	Game      string     `json:"game"`
	Type      string     `json:"type"` // manual | hotkey | pre-restore
	Files     []FileInfo `json:"files"`
	Timestamp string     `json:"timestamp"`
	TotalSize int64      `json:"totalSize"`
}

const metaFileName = "meta.json"

// Manager backs up and restores a single source folder for one game profile.
// The entire Source folder is treated as one backup unit; there is no slot
// auto-detection, which keeps the tool universal across games.
type Manager struct {
	Game       string
	Source     string
	BackupRoot string
	now        func() time.Time
}

// New creates a Manager. If now is nil, time.Now is used.
func New(game, source, backupRoot string, now func() time.Time) (*Manager, error) {
	if source == "" || backupRoot == "" {
		return nil, errors.New("source and backupRoot are required")
	}
	absSrc, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	absBak, err := filepath.Abs(backupRoot)
	if err != nil {
		return nil, err
	}
	if nested(absSrc, absBak) || nested(absBak, absSrc) {
		return nil, errors.New("backupRoot and source must not be nested within each other")
	}
	if now == nil {
		now = time.Now
	}
	return &Manager{Game: game, Source: absSrc, BackupRoot: absBak, now: now}, nil
}

// nested reports whether child is inside parent (or equal).
func nested(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." && !filepath.IsAbs(rel))
}

// validName rejects empty names and path traversal for timestamps.
func validName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.Contains(name, "..") {
		return fmt.Errorf("invalid name: %q", name)
	}
	return nil
}

// Create makes a backup of the entire Source folder with a note and type.
func (m *Manager) Create(note, typ string) (*Meta, error) {
	if typ == "" {
		typ = "manual"
	}
	info, err := os.Stat(m.Source)
	if err != nil {
		return nil, fmt.Errorf("source folder not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source is not a directory: %s", m.Source)
	}

	ts := m.now().Format("20060102-150405")
	dest := filepath.Join(m.BackupRoot, ts)
	for i := 1; ; i++ {
		if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
			break
		}
		dest = filepath.Join(m.BackupRoot, fmt.Sprintf("%s-%d", ts, i))
	}

	tmp := dest + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, err
	}
	files, total, err := copyTree(m.Source, tmp)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}

	meta := &Meta{
		Note:      note,
		CreatedAt: m.now(),
		Game:      m.Game,
		Type:      typ,
		Files:     files,
		Timestamp: filepath.Base(dest),
		TotalSize: total,
	}
	if err := writeMeta(tmp, meta); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.RemoveAll(tmp)
		return nil, err
	}
	return meta, nil
}

// List returns all backups, newest first.
func (m *Manager) List() ([]*Meta, error) {
	entries, err := os.ReadDir(m.BackupRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []*Meta{}, nil
		}
		return nil, err
	}
	var metas []*Meta
	for _, e := range entries {
		if !e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		meta, err := readMeta(filepath.Join(m.BackupRoot, e.Name()))
		if err != nil {
			meta = &Meta{Timestamp: e.Name(), Type: "unknown"}
		}
		meta.Timestamp = e.Name()
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Timestamp > metas[j].Timestamp })
	return metas, nil
}

// Delete removes a single backup.
func (m *Manager) Delete(timestamp string) error {
	if err := validName(timestamp); err != nil {
		return err
	}
	target := filepath.Join(m.BackupRoot, timestamp)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}
	return os.RemoveAll(target)
}

// UpdateNote changes the note of an existing backup.
func (m *Manager) UpdateNote(timestamp, note string) error {
	if err := validName(timestamp); err != nil {
		return err
	}
	dir := filepath.Join(m.BackupRoot, timestamp)
	meta, err := readMeta(dir)
	if err != nil {
		return err
	}
	meta.Note = note
	return writeMeta(dir, meta)
}

// Restore overwrites the Source folder with the contents of a backup.
// It first creates a pre-restore safety snapshot of the current state.
func (m *Manager) Restore(timestamp string) (safety *Meta, err error) {
	if err := validName(timestamp); err != nil {
		return nil, err
	}
	backupDir := filepath.Join(m.BackupRoot, timestamp)
	if _, err := os.Stat(backupDir); err != nil {
		return nil, fmt.Errorf("backup not found: %w", err)
	}

	dst := m.Source

	if _, statErr := os.Stat(dst); statErr == nil {
		safety, err = m.Create("自动安全快照（恢复前）", "pre-restore")
		if err != nil {
			return nil, fmt.Errorf("pre-restore snapshot failed: %w", err)
		}
	}

	parent := filepath.Dir(dst)
	staging, err := os.MkdirTemp(parent, ".sg-restore-")
	if err != nil {
		return safety, err
	}
	defer os.RemoveAll(staging)

	if _, _, err := copyTreeSkip(backupDir, staging, metaFileName); err != nil {
		return safety, err
	}

	old := dst + ".sg-old"
	_ = os.RemoveAll(old)
	if _, statErr := os.Stat(dst); statErr == nil {
		if err := os.Rename(dst, old); err != nil {
			return safety, err
		}
	}
	if err := os.Rename(staging, dst); err != nil {
		_ = os.Rename(old, dst)
		return safety, err
	}
	_ = os.RemoveAll(old)
	return safety, nil
}

// copyTree copies all files from src into dst, returning file metadata.
func copyTree(src, dst string) ([]FileInfo, int64, error) {
	return copyTreeSkip(src, dst, "")
}

// copyTreeSkip copies src into dst, skipping a top-level file named skip.
func copyTreeSkip(src, dst, skip string) ([]FileInfo, int64, error) {
	var files []FileInfo
	var total int64
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if skip != "" && rel == skip {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		sum, size, err := copyFile(path, target)
		if err != nil {
			return err
		}
		files = append(files, FileInfo{Path: filepath.ToSlash(rel), Size: size, SHA256: sum})
		total += size
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, total, err
}

// copyFile copies a single file and returns its sha256 and size.
func copyFile(src, dst string) (string, int64, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", 0, err
	}
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", 0, err
	}
	defer out.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), in)
	if err != nil {
		return "", 0, err
	}
	if err := out.Sync(); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func writeMeta(dir string, meta *Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, metaFileName), data, 0o644)
}

func readMeta(dir string) (*Meta, error) {
	data, err := os.ReadFile(filepath.Join(dir, metaFileName))
	if err != nil {
		return nil, err
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
