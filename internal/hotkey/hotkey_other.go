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
