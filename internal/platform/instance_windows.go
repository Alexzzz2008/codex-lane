//go:build windows

package platform

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32DLL       = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW = user32DLL.NewProc("MessageBoxW")
)

const (
	mbOK              = 0x00000000
	mbIconInformation = 0x00000040
	mbSetForeground   = 0x00010000
	mbTopmost         = 0x00040000
)

// ProcessAlive reports whether pid refers to a running process.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

// ShowNotice displays a blocking message box. It is used for one-shot startup
// notices before the window exists.
func ShowNotice(title, message string) {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	messagePtr, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	_, _, _ = procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOK|mbIconInformation|mbSetForeground|mbTopmost),
	)
}
