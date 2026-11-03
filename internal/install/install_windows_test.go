//go:build windows

package install

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

func TestAdoptInstallsExecutableAndShortcut(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "local")
	desktop := filepath.Join(root, "desktop")
	if err := os.MkdirAll(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("CODEXLANE_DESKTOP_DIR", desktop)

	source := filepath.Join(desktop, "CodexLane.exe")
	if err := os.WriteFile(source, []byte("portable-build"), 0o755); err != nil {
		t.Fatal(err)
	}

	target, err := Adopt(source)
	if err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	want := filepath.Join(local, "Programs", "CodexLane", "CodexLane.exe")
	if target != want {
		t.Fatalf("target = %s, want %s", target, want)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "portable-build" {
		t.Fatalf("installed executable = %q, err = %v", content, err)
	}
	if !Managed(target) {
		t.Fatal("installed executable should be managed")
	}

	shortcut, err := ShortcutPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shortcut); err != nil {
		t.Fatalf("shortcut missing: %v", err)
	}
	if got := readShortcutTarget(t, shortcut); !samePath(got, target) {
		t.Fatalf("shortcut target = %s, want %s", got, target)
	}
}

func readShortcutTarget(t *testing.T, shortcut string) string {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initialized, err := initializeOle()
	if err != nil {
		t.Fatalf("CoInitializeEx: %v", err)
	}
	if initialized {
		defer ole.CoUninitialize()
	}
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		t.Fatalf("WScript.Shell: %v", err)
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Release()
	created, err := oleutil.CallMethod(shell, "CreateShortcut", shortcut)
	if err != nil {
		t.Fatal(err)
	}
	dispatch := created.ToIDispatch()
	defer dispatch.Release()
	target, err := oleutil.GetProperty(dispatch, "TargetPath")
	if err != nil {
		t.Fatal(err)
	}
	return target.ToString()
}
