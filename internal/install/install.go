// Package install keeps the running build in a stable directory.
//
// Self updates replace the executable on disk. When that executable sits on the
// desktop, Explorer sees a brand new file and drops its icon into the first free
// slot. Adopting a portable build into a fixed directory (the same model Magpie
// uses with its zip releases) leaves only a shortcut on the desktop, so updates
// never move anything the user arranged.
package install

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/platform"
)

const (
	replacesEnv     = "CODEXLANE_REPLACES"
	removeSourceEnv = "CODEXLANE_REMOVE_SOURCE"
	disableEnv      = "CODEXLANE_NO_MIGRATE"
)

// Directory is the stable per-user location that owns the real executable.
func Directory() (string, error) {
	base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if base == "" {
		return "", errors.New("LOCALAPPDATA 未设置")
	}
	return filepath.Join(base, "Programs", "CodexLane"), nil
}

// TargetExecutable is the path self updates rewrite in place.
func TargetExecutable() (string, error) {
	directory, err := Directory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "CodexLane.exe"), nil
}

// Managed reports whether executable already runs from the install directory.
func Managed(executable string) bool {
	directory, err := Directory()
	if err != nil {
		return false
	}
	return samePath(filepath.Dir(executable), directory)
}

// PortableLocation reports whether the build runs from somewhere a release zip
// is usually extracted into, which is exactly where icon layout breaks.
func PortableLocation(executable string) bool {
	directory := filepath.Dir(filepath.Clean(executable))
	if desktop, err := DesktopDirectory(); err == nil && samePath(directory, desktop) {
		return true
	}
	home, err := os.UserHomeDir()
	if err == nil {
		for _, name := range []string{"Desktop", "Downloads"} {
			if samePath(directory, filepath.Join(home, name)) {
				return true
			}
		}
	}
	separator := string(os.PathSeparator)
	lower := strings.ToLower(directory)
	return strings.Contains(lower, separator+"temp"+separator) || strings.HasSuffix(lower, separator+"temp")
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

// Adopt copies the running build into the install directory and points a
// desktop shortcut at it.
func Adopt(executable string) (string, error) {
	target, err := TargetExecutable()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("创建安装目录失败：%w", err)
	}
	if !samePath(executable, target) {
		if err := copyExecutable(executable, target); err != nil {
			return "", err
		}
	}
	if _, err := CreateShortcut(target); err != nil {
		platform.AppLog("创建桌面快捷方式失败：%v", err)
	}
	return target, nil
}

// AutoMigrate hands a portable build over to the install directory and returns
// true when a migrated copy took over, meaning the caller should exit.
func AutoMigrate() bool {
	if strings.TrimSpace(os.Getenv(disableEnv)) != "" {
		return false
	}
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	if Managed(executable) || !PortableLocation(executable) {
		return false
	}
	if anotherLaneRunning() {
		// Another build owns the window right now. Migrating here would leave
		// the user with two copies and no visible window, so let the normal
		// single-instance notice explain what to do instead.
		return false
	}
	target, err := Adopt(executable)
	if err != nil {
		platform.AppLog("迁移到本机目录失败：%v", err)
		return false
	}
	if err := Launch(target, executable); err != nil {
		platform.AppLog("启动迁移后的程序失败：%v", err)
		return false
	}
	platform.AppLog("已迁移到本机目录：%s（原位置 %s）", target, executable)
	return true
}

func anotherLaneRunning() bool {
	record, ok := platform.ReadInstance()
	if !ok || record.PID <= 0 || record.PID == os.Getpid() {
		return false
	}
	return platform.ProcessAlive(record.PID) && looksLikeLane(record.Executable)
}

func looksLikeLane(executable string) bool {
	base := strings.ToLower(filepath.Base(executable))
	return strings.Contains(base, "codexlane")
}

// RemovePending deletes the portable copy that handed over to this build. It
// must run after the previous process exited and released its image file.
func RemovePending() {
	source := strings.TrimSpace(os.Getenv(removeSourceEnv))
	if source == "" {
		return
	}
	_ = os.Unsetenv(removeSourceEnv)
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := os.Remove(source)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			platform.AppLog("已移除原先的便携副本：%s", source)
			return
		}
		if time.Now().After(deadline) {
			platform.AppLog("移除便携副本失败：%v", err)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func copyExecutable(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("读取当前程序失败：%w", err)
	}
	defer input.Close()

	staged := target + ".new"
	_ = os.Remove(staged)
	output, err := os.OpenFile(staged, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("写入安装目录失败：%w", err)
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("复制程序失败：%w", err)
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("安装程序失败：%w", err)
	}
	return nil
}
