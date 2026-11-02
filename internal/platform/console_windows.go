//go:build windows

package platform

import (
	"os"
	"syscall"
)

const (
	stdInputHandle     = ^uint32(9)
	stdOutputHandle    = ^uint32(10)
	stdErrorHandle     = ^uint32(11)
	attachParentPID    = ^uint32(0)
	invalidHandleValue = ^uintptr(0)
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole    = kernel32.NewProc("AttachConsole")
	procAllocConsole     = kernel32.NewProc("AllocConsole")
	procGetStdHandle     = kernel32.NewProc("GetStdHandle")
	procSetConsoleCP     = kernel32.NewProc("SetConsoleCP")
	procSetConsoleOutput = kernel32.NewProc("SetConsoleOutputCP")
)

// PrepareConsole makes a windowsgui executable usable from a terminal for
// explicit command-line modes. Redirected standard handles are left untouched.
func PrepareConsole() {
	if validStdHandle(stdOutputHandle) || validStdHandle(stdErrorHandle) {
		return
	}
	attached, _, _ := procAttachConsole.Call(uintptr(attachParentPID))
	if attached == 0 {
		allocated, _, _ := procAllocConsole.Call()
		if allocated == 0 {
			return
		}
	}
	_, _, _ = procSetConsoleCP.Call(65001)
	_, _, _ = procSetConsoleOutput.Call(65001)
	if file, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = file
		os.Stderr = file
	}
	if file, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = file
	}
}

func validStdHandle(which uint32) bool {
	handle, _, _ := procGetStdHandle.Call(uintptr(which))
	return handle != 0 && handle != invalidHandleValue
}
