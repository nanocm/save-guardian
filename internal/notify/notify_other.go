//go:build !windows

package notify

// Beep is a no-op on non-Windows platforms.
func Beep(ok bool) {}
