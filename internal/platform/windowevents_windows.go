//go:build windows

package platform

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSetWinEventHook   = user32DLL.NewProc("SetWinEventHook")
	procUnhookWinEvent    = user32DLL.NewProc("UnhookWinEvent")
	procGetMessage        = user32DLL.NewProc("GetMessageW")
	procPeekMessage       = user32DLL.NewProc("PeekMessageW")
	procPostThreadMessage = user32DLL.NewProc("PostThreadMessageW")
	procTranslateMessage  = user32DLL.NewProc("TranslateMessage")
	procDispatchMessage   = user32DLL.NewProc("DispatchMessageW")
	procWindowProcessID   = user32DLL.NewProc("GetWindowThreadProcessId")
	windowEventHooks      sync.Map
	windowEventCallback   = syscall.NewCallback(func(hook uintptr, event uint32, window uintptr, object, child int32, thread, timestamp uint32) uintptr {
		if window == 0 || object != 0 || child != 0 {
			return 0
		}
		value, ok := windowEventHooks.Load(hook)
		if !ok {
			return 0
		}
		var processID uint32
		procWindowProcessID.Call(window, uintptr(unsafe.Pointer(&processID)))
		if processID != 0 {
			select {
			case value.(chan<- int) <- int(processID):
			default:
			}
		}
		return 0
	})
)

type windowEventMessage struct {
	window  uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   struct{ x, y int32 }
	private uint32
}

func WatchWindowProcesses(ctx context.Context, events chan<- int) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ctx.Err(); err != nil {
		return err
	}
	var message windowEventMessage
	procPeekMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 0)
	for _, event := range []uintptr{0x0003, 0x8002} {
		hook, _, _ := procSetWinEventHook.Call(event, event, 0, windowEventCallback, 0, 0, 0)
		if hook == 0 {
			return errors.New("无法注册窗口事件；仍可在总览手动接入 Codex")
		}
		windowEventHooks.Store(hook, events)
		defer func() {
			procUnhookWinEvent.Call(hook)
			windowEventHooks.Delete(hook)
		}()
	}
	threadID := windows.GetCurrentThreadId()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			procPostThreadMessage.Call(uintptr(threadID), 0x0012, 0, 0)
		case <-done:
		}
	}()
	for {
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return errors.New("窗口事件监听已退出；仍可在总览手动接入 Codex")
		}
		if result == 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func ProcessExecutable(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 512)
	size := uint32(len(buffer))
	err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
	if errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		buffer = make([]uint16, 32768)
		size = uint32(len(buffer))
		err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
	}
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer[:size]), nil
}
