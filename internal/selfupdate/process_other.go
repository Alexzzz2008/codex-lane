//go:build !windows

package selfupdate

import (
	"os/exec"
	"syscall"
	"time"
)

func detach(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setsid = true
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func sharingViolation(error) bool {
	return false
}

func trialStart(string) error {
	return nil
}

func hideFile(string) {}

func unhideFile(string) {}

func WaitInstanceFree(string, time.Duration) error { return nil }
