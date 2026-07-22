//go:build !windows

package folderpick

// Pick is a no-op on non-Windows platforms. The web UI also lets users paste a
// path manually, so folder picking is a convenience only.
func Pick() (string, error) {
	return "", nil
}
