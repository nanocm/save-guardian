//go:build windows

package hotkey

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHK         = user32.NewProc("RegisterHotKey")
	procUnregisterHK       = user32.NewProc("UnregisterHotKey")
	procGetMessage         = user32.NewProc("GetMessageW")
	procPostThreadMsg      = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

const (
	wmHotkey = 0x0312
	wmApp    = 0x8000 // WM_APP: our custom "re-arm the hotkey" signal
	hkID     = 1
)

// hkState lets Rearm reach the listener's message-loop thread.
var (
	hkMu          sync.Mutex
	hkThreadID    uintptr
	hkPending     string
	hkHavePending bool
)

// Listen registers the hotkey and invokes onTrigger each time it fires. It
// blocks on a Windows message loop, so callers should run it in a goroutine.
// While running it also honors Rearm requests to switch to a new hotkey
// without restarting the program.
func Listen(spec string, onTrigger func()) error {
	cur, err := Parse(spec)
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid, _, _ := procGetCurrentThreadID.Call()
	hkMu.Lock()
	hkThreadID = tid
	hkMu.Unlock()
	defer func() {
		hkMu.Lock()
		hkThreadID = 0
		hkMu.Unlock()
	}()

	r, _, e := procRegisterHK.Call(0, hkID, uintptr(cur.Mods), uintptr(cur.VK))
	if r == 0 {
		return fmt.Errorf("RegisterHotKey failed: %v", e)
	}
	defer procUnregisterHK.Call(0, hkID)

	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			return nil
		}
		switch m.message {
		case wmHotkey:
			onTrigger()
		case wmApp:
			cur = rearmLocked(cur)
		}
	}
}

// rearmLocked applies a pending hotkey change on the message-loop thread. If
// registering the new hotkey fails, it restores the previous binding so the
// user is never left without a working hotkey.
func rearmLocked(cur Parsed) Parsed {
	hkMu.Lock()
	spec := hkPending
	have := hkHavePending
	hkHavePending = false
	hkMu.Unlock()
	if !have {
		return cur
	}
	np, err := Parse(spec)
	if err != nil {
		return cur
	}
	procUnregisterHK.Call(0, hkID)
	if r, _, _ := procRegisterHK.Call(0, hkID, uintptr(np.Mods), uintptr(np.VK)); r == 0 {
		// Re-registering the new key failed; restore the previous one.
		procRegisterHK.Call(0, hkID, uintptr(cur.Mods), uintptr(cur.VK))
		return cur
	}
	return np
}

// Rearm validates spec and asks the running listener to switch to it. It
// returns an error if the spec is invalid or no listener is running.
func Rearm(spec string) error {
	if _, err := Parse(spec); err != nil {
		return err
	}
	hkMu.Lock()
	tid := hkThreadID
	hkPending = spec
	hkHavePending = true
	hkMu.Unlock()
	if tid == 0 {
		return errors.New("hotkey listener is not running")
	}
	if r, _, e := procPostThreadMsg.Call(tid, wmApp, 0, 0); r == 0 {
		return fmt.Errorf("PostThreadMessage failed: %v", e)
	}
	return nil
}
