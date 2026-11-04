//go:build windows

package platform

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startWindowEventTest(t *testing.T) (<-chan int, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan int, 16)
	done := make(chan error, 1)
	go func() { done <- WatchWindowProcesses(ctx, events) }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		count := 0
		windowEventHooks.Range(func(key, value any) bool { count++; return true })
		if count >= 2 {
			return events, cancel, done
		}
		select {
		case err := <-done:
			cancel()
			t.Fatalf("window watcher exited before initialization: %v", err)
		case <-deadline.C:
			cancel()
			t.Fatal("window hooks were not registered")
		case <-tick.C:
		}
	}
}

func TestNativeWindowEventDeliveryAndCancellation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	events, cancel, done := startWindowEventTest(t)
	defer cancel()
	class, _ := windows.UTF16PtrFromString("STATIC")
	window, _, err := user32DLL.NewProc("CreateWindowExW").Call(0x08000080, uintptr(unsafe.Pointer(class)), 0, 0x80000000, 10000, 10000, 1, 1, 0, 0, 0, 0)
	if window == 0 {
		t.Fatalf("create hidden test window: %v", err)
	}
	defer user32DLL.NewProc("DestroyWindow").Call(window)
	user32DLL.NewProc("ShowWindow").Call(window, 4)
	user32DLL.NewProc("NotifyWinEvent").Call(0x8002, window, 0, 0)
	deadline := time.After(time.Second)
	for {
		select {
		case pid := <-events:
			if pid == os.Getpid() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("watcher did not release its message thread")
				}
				return
			}
		case <-deadline:
			t.Fatal("native window notification was not received")
		}
	}
}

func TestProcessExecutableUsesNativeQuery(t *testing.T) {
	actual, err := ProcessExecutable(os.Getpid())
	expected, _ := os.Executable()
	if err != nil || !strings.EqualFold(actual, expected) {
		t.Fatalf("process path: %s, error=%v", actual, err)
	}
}

func processCPUTime(t *testing.T) time.Duration {
	t.Helper()
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	ticks := (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) +
		(uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
	return time.Duration(ticks) * 100 * time.Nanosecond
}

func TestWindowWatcherIdleCPU(t *testing.T) {
	started := processCPUTime(t)
	time.Sleep(time.Second)
	baseline := processCPUTime(t) - started
	_, cancel, done := startWindowEventTest(t)
	defer cancel()
	started = processCPUTime(t)
	time.Sleep(time.Second)
	idle := processCPUTime(t) - started
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop")
	}
	t.Logf("idle CPU per second: baseline=%s, native watcher=%s", baseline, idle)
	if idle > baseline+40*time.Millisecond {
		t.Fatalf("idle window watcher exceeded CPU allowance: %s", idle)
	}
}
