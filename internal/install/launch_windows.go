//go:build windows

package install

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// Launch starts the installed build, tells it which process to wait for and
// which portable copy to delete once that process is gone.
func Launch(target, source string) error {
	command := exec.Command(target)
	command.Env = append(os.Environ(),
		replacesEnv+"="+strconv.Itoa(os.Getpid()),
		removeSourceEnv+"="+source,
	)
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	return command.Start()
}
