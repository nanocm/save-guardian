//go:build windows

package folderpick

import (
	"syscall"
	"unsafe"
)

var (
	shell32               = syscall.NewLazyDLL("shell32.dll")
	ole32                 = syscall.NewLazyDLL("ole32.dll")
	user32                = syscall.NewLazyDLL("user32.dll")
	procSHBrowseForFolder = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDL  = shell32.NewProc("SHGetPathFromIDListW")
	procCoInitialize      = ole32.NewProc("CoInitialize")
	procCoTaskMemFree     = ole32.NewProc("CoTaskMemFree")
	procSetForegroundWin  = user32.NewProc("SetForegroundWindow")
	procSetWindowPos      = user32.NewProc("SetWindowPos")
)

type browseInfo struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

const (
	bifReturnOnlyFSDirs = 0x00000001
	bifNewDialogStyle   = 0x00000040
	maxPath             = 260

	bffmInitialized = 1

	// SetWindowPos: HWND_TOPMOST then HWND_NOTOPMOST to force to front.
	hwndTopmost   = ^uintptr(0)     // (HWND)-1
	hwndNoTopmost = ^uintptr(0) - 1 // (HWND)-2
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpShowWindow = 0x0040
)

// browseCallback runs inside the folder dialog. On initialization it forces the
// dialog to the foreground so it never hides behind the game or other windows.
func browseCallback(hwnd uintptr, uMsg uint32, lParam, lpData uintptr) uintptr {
	if uMsg == bffmInitialized {
		procSetForegroundWin.Call(hwnd)
		procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoSize|swpNoMove|swpShowWindow)
		procSetWindowPos.Call(hwnd, hwndNoTopmost, 0, 0, 0, 0, swpNoSize|swpNoMove|swpShowWindow)
	}
	return 0
}

// Pick opens the native Windows folder chooser and returns the selected path,
// or an empty string if the user cancels.
func Pick() (string, error) {
	procCoInitialize.Call(0)

	title, _ := syscall.UTF16PtrFromString("选择文件夹")
	display := make([]uint16, maxPath)
	bi := browseInfo{
		lpszTitle:      title,
		pszDisplayName: &display[0],
		ulFlags:        bifReturnOnlyFSDirs | bifNewDialogStyle,
		lpfn:           syscall.NewCallback(browseCallback),
	}
	pidl, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil // user cancelled
	}
	defer procCoTaskMemFree.Call(pidl)

	buf := make([]uint16, maxPath)
	ret, _, _ := procSHGetPathFromIDL.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	if ret == 0 {
		return "", nil
	}
	return syscall.UTF16ToString(buf), nil
}
