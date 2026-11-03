//go:build !windows

package platform

import (
	"fmt"
	"os"
	"syscall"
)

// ProcessAlive reports whether pid refers to a running process.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// ShowNotice prints the notice; non-Windows builds have no message box.
func ShowNotice(title, message string) {
	fmt.Printf("%s: %s\n", title, message)
}
