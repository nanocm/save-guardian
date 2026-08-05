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

// Verify re-hashes every file recorded in a backup's meta.json and returns the
// relative paths whose size or SHA-256 no longer matches (or that are missing).
// An empty slice means the backup is intact.
func (m *Manager) Verify(timestamp string) ([]string, error) {
	if err := validName(timestamp); err != nil {
		return nil, err
	}
	dir := filepath.Join(m.BackupRoot, timestamp)
	meta, err := readMeta(dir)
	if err != nil {
		return nil, err
	}
	var bad []string
	for _, f := range meta.Files {
		sum, size, err := hashFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
		if err != nil || size != f.Size || sum != f.SHA256 {
			bad = append(bad, f.Path)
		}
	}
	return bad, nil
}

// Restore overwrites the Source folder with the contents of a backup.
// It verifies the backup's checksums first, then creates a pre-restore safety
// snapshot of the current state before overwriting.
func (m *Manager) Restore(timestamp string) (safety *Meta, err error) {
	if err := validName(timestamp); err != nil {
		return nil, err
	}
	backupDir := filepath.Join(m.BackupRoot, timestamp)
	if _, err := os.Stat(backupDir); err != nil {
		return nil, fmt.Errorf("backup not found: %w", err)
	}

	// Refuse to restore a corrupted backup over the live save.
	bad, err := m.Verify(timestamp)
	if err != nil {
		return nil, fmt.Errorf("integrity check failed: %w", err)
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("备份已损坏，校验和不匹配（%d 个文件）：%s", len(bad), strings.Join(bad, ", "))
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

	dirty, err := m.replaceDir(staging, dst)
	if err == nil {
		return safety, nil
	}
	// The in-place fallback may have written some files before failing, leaving
	// the save as a mix of old and new. Put the pre-restore state back so the
	// player is never left with an inconsistent save they must repair by hand.
	if dirty && safety != nil {
		snapshot := filepath.Join(m.BackupRoot, safety.Timestamp)
		if rbErr := overwriteInPlace(snapshot, dst, metaFileName); rbErr == nil {
			return safety, fmt.Errorf("恢复失败，已自动回滚到恢复前的状态：%w", err)
		}
		return safety, fmt.Errorf("恢复失败，且自动回滚未完成，存档可能是新旧混合状态。"+
			"请先不要进入游戏，待游戏完全退出后用「安全快照 %s」再恢复一次：%w", safety.Timestamp, err)
	}
	return safety, err
}

// replaceDir makes dst hold exactly what staging holds.
//
// The fast path is an atomic swap (move dst aside, move staging into place) so
// that an interrupted restore can never leave a half-written save. On Windows,
// however, renaming a directory fails with "Access is denied" while any process
// holds a handle to it or to a file inside it — the game sitting at its title
// screen, an antivirus scanner, the search indexer, cloud sync, or an open
// Explorer window. Those locks are typically transient, so retry briefly and
// then fall back to overwriting dst's contents file by file: less atomic, but
// per-file writes succeed where a directory rename cannot, and the pre-restore
// snapshot already protects the previous state.
// It reports dirty=true when the fallback may have partially written dst, so
// the caller can roll back to the pre-restore snapshot.
func (m *Manager) replaceDir(staging, dst string) (dirty bool, err error) {
	m.cleanupOld(dst)
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}
		// The atomic swap either fully succeeds or rolls itself back, so it
		// never leaves dst dirty.
		if err := m.swapDir(staging, dst); err == nil {
			return false, nil
		} else {
			lastErr = err
		}
	}
	if err := overwriteInPlace(staging, dst, ""); err != nil {
		return true, fmt.Errorf("原子替换失败（%v）；就地覆盖也未成功：%w"+
			"（请确认已完全退出游戏，并暂时关闭杀毒软件或云同步后重试）", lastErr, err)
	}
	return false, nil
}

// swapDir performs one atomic swap attempt, putting the live folder back if the
// second rename fails. Each attempt uses a fresh sidelined name so a leftover
// locked directory from an earlier failure can never block future restores.
func (m *Manager) swapDir(staging, dst string) error {
	old := dst + ".sg-old-" + m.now().Format("20060102-150405")
	moved := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			return err
		}
		moved = true
	}
	if err := os.Rename(staging, dst); err != nil {
		if moved {
			_ = os.Rename(old, dst) // restore the live save
		}
		return err
	}
	if moved {
		_ = os.RemoveAll(old)
	}
	return nil
}

// cleanupOld best-effort removes sidelined folders left behind by earlier
// restores whose cleanup was blocked by a file lock, so old save copies do not
// accumulate next to the live save.
func (m *Manager) cleanupOld(dst string) {
	matches, err := filepath.Glob(dst + ".sg-old-*")
	if err != nil {
		return
	}
	for _, p := range matches {
		_ = os.RemoveAll(p)
	}
}

// overwriteInPlace makes dst match src by writing over existing files and
// deleting whatever src does not contain, skipping a top-level file named skip.
// This is the non-atomic fallback used when dst cannot be renamed, and also the
// rollback path when that fallback fails part-way.
func overwriteInPlace(src, dst, skip string) error {
	want := make(map[string]bool)
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
		want[rel] = true
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		_, _, err = copyFile(path, target)
		return err
	})
	if err != nil {
		return err
	}
	// Prune files from the previous save that this backup does not contain.
	var stale []string
	_ = filepath.Walk(dst, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(dst, path)
		if relErr != nil || rel == "." || want[rel] {
			return nil
		}
		stale = append(stale, path)
		return nil
	})
	// Delete deepest paths first so directories are empty by the time we
	// remove them.
	sort.Slice(stale, func(i, j int) bool { return len(stale[i]) > len(stale[j]) })
	for _, p := range stale {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
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

// hashFile returns the sha256 and size of a file without copying it.
func hashFile(path string) (string, int64, error) {
	in, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	h := sha256.New()
	n, err := io.Copy(h, in)
	if err != nil {
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
