//go:build windows

package hotkey

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procRegisterHK   = user32.NewProc("RegisterHotKey")
	procUnregisterHK = user32.NewProc("UnregisterHotKey")
	procGetMessage   = user32.NewProc("GetMessageW")
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

const wmHotkey = 0x0312

// Listen registers the hotkey and invokes onTrigger each time it fires.
// It blocks on a Windows message loop, so callers should run it in a goroutine.
func Listen(spec string, onTrigger func()) error {
	p, err := Parse(spec)
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const id = 1
	r, _, e := procRegisterHK.Call(0, uintptr(id), uintptr(p.Mods), uintptr(p.VK))
	if r == 0 {
		return fmt.Errorf("RegisterHotKey failed: %v", e)
	}
	defer procUnregisterHK.Call(0, uintptr(id))

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			return nil
		}
		if m.message == wmHotkey {
			onTrigger()
		}
	}
}
