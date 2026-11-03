//go:build windows

package platform

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnsureSelfVisible clears a hidden attribute on the running executable.
// Updates stage the new build as a hidden file, and an interrupted install can
// leave the installed executable hidden, which makes it vanish in Explorer.
func EnsureSelfVisible() {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	name, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return
	}
	attributes, err := windows.GetFileAttributes(name)
	if err != nil || attributes&windows.FILE_ATTRIBUTE_HIDDEN == 0 {
		return
	}
	_ = windows.SetFileAttributes(name, windows.FILE_ATTRIBUTE_NORMAL)
}
