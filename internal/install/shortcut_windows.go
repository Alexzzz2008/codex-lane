//go:build windows

package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

const shortcutName = "Codex Lane.lnk"

// DesktopDirectory resolves the real desktop folder, including OneDrive
// redirection.
func DesktopDirectory() (string, error) {
	if override := strings.TrimSpace(os.Getenv("CODEXLANE_DESKTOP_DIR")); override != "" {
		return filepath.Clean(override), nil
	}
	path, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

// ShortcutPath is the desktop entry that survives every update.
func ShortcutPath() (string, error) {
	desktop, err := DesktopDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(desktop, shortcutName), nil
}

// CreateShortcut writes a shortcut that points at the installed executable.
func CreateShortcut(target string) (string, error) {
	// COM is apartment-scoped per OS thread; keep the goroutine pinned so the
	// initialization and the uninitialization always hit the same thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	path, err := ShortcutPath()
	if err != nil {
		return "", err
	}
	initialized, err := initializeOle()
	if err != nil {
		return "", fmt.Errorf("初始化 Shell 组件失败：%w", err)
	}
	if initialized {
		defer ole.CoUninitialize()
	}

	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return "", fmt.Errorf("加载 WScript.Shell 失败：%w", err)
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return "", err
	}
	defer shell.Release()

	created, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		return "", fmt.Errorf("创建快捷方式失败：%w", err)
	}
	shortcut := created.ToIDispatch()
	defer shortcut.Release()

	_, _ = oleutil.PutProperty(shortcut, "TargetPath", target)
	_, _ = oleutil.PutProperty(shortcut, "WorkingDirectory", filepath.Dir(target))
	_, _ = oleutil.PutProperty(shortcut, "IconLocation", target+",0")
	_, _ = oleutil.PutProperty(shortcut, "Description", "Codex Lane")
	if _, err := oleutil.CallMethod(shortcut, "Save"); err != nil {
		return "", fmt.Errorf("保存快捷方式失败：%w", err)
	}
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

// initializeOle reports whether this call owns the COM initialization. S_FALSE
// means COM is already initialized on this thread by an earlier caller.
func initializeOle() (bool, error) {
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	if err == nil {
		return true, nil
	}
	var oleErr *ole.OleError
	if errors.As(err, &oleErr) && oleErr.Code() == 1 {
		return false, nil
	}
	return false, err
}
