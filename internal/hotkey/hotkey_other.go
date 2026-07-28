//go:build !windows

package hotkey

// Listen is a no-op on non-Windows platforms so the project builds and tests
// cross-platform. Real global hotkey support is Windows-only.
func Listen(spec string, onTrigger func()) error {
	if _, err := Parse(spec); err != nil {
		return err
	}
	select {} // block forever without firing
}

// Rearm validates the spec on non-Windows platforms but is otherwise a no-op,
// since there is no global hotkey to re-register.
func Rearm(spec string) error {
	_, err := Parse(spec)
	return err
}
