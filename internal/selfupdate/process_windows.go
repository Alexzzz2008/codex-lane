//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	detachedProcess = 0x00000008
	newProcessGroup = 0x00000200
	createSuspended = 0x00000004
	createNoWindow  = 0x08000000
)

func detach(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= detachedProcess | newProcessGroup
}

func processAlive(pid int) bool {
	handle, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	event, _ := syscall.WaitForSingleObject(handle, 0)
	return event == syscall.WAIT_TIMEOUT
}

func sharingViolation(err error) bool {
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}

func trialStart(path string) error {
	command := exec.Command(path)
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createSuspended | createNoWindow,
	}
	if err := command.Start(); err != nil {
		return err
	}
	_ = command.Process.Kill()
	_ = command.Wait()
	return nil
}

func hideFile(path string) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	_ = windows.SetFileAttributes(name, windows.FILE_ATTRIBUTE_HIDDEN)
}

func unhideFile(path string) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	_ = windows.SetFileAttributes(name, windows.FILE_ATTRIBUTE_NORMAL)
}

// WaitInstanceFree blocks until no process owns the named Wails single-instance
// mutex, which is the real gate for showing a window.
func WaitInstanceFree(name string, timeout time.Duration) error {
	pointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		handle, err := windows.CreateMutex(nil, false, pointer)
		if err == nil {
			windows.CloseHandle(handle)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("previous instance still holds %s", name)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
