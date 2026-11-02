package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
)

func KillMainProcess(ctx context.Context, installLocation string, pid int) error {
	processes, err := ListProcesses(ctx, installLocation)
	if err != nil {
		return err
	}
	for _, process := range processes {
		if process.PID == pid && process.IsMain {
			proc, findErr := os.FindProcess(pid)
			if findErr != nil {
				return findErr
			}
			if killErr := proc.Kill(); killErr != nil {
				return fmt.Errorf("stop Codex process %d: %w", pid, killErr)
			}
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) {
				current, listErr := ListProcesses(ctx, installLocation)
				if listErr == nil {
					found := false
					for _, candidate := range current {
						if candidate.PID == pid {
							found = true
							break
						}
					}
					if !found {
						return nil
					}
				}
				time.Sleep(250 * time.Millisecond)
			}
			return errors.New("Codex did not exit within 8 seconds")
		}
	}
	return errors.New("target Codex main process is no longer running")
}

func Launch(pkg PackageInfo, engine string, interfaceProxy string, kernelProxy string, noProxy string, home string) (model.LaunchResult, error) {
	if err := ValidateCodexPath(pkg.ExecutablePath, pkg.InstallLocation); err != nil {
		return model.LaunchResult{}, err
	}
	args := []string{
		"--proxy-server=" + interfaceProxy,
		"--disable-quic",
	}
	cmd := exec.Command(pkg.ExecutablePath, args...)
	cmd.Dir = home
	cmd.Env = mergeEnvironment(os.Environ(), map[string]string{
		"HTTP_PROXY":  kernelProxy,
		"HTTPS_PROXY": kernelProxy,
		"ALL_PROXY":   kernelProxy,
		"NO_PROXY":    noProxy,
	})
	platform.ConfigureDetached(cmd)
	if err := cmd.Start(); err != nil {
		return model.LaunchResult{}, fmt.Errorf("start Codex: %w", err)
	}
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
		_ = cmd.Process.Release()
	}
	return model.LaunchResult{
		Started:     true,
		PID:         pid,
		CommandLine: pkg.ExecutablePath + " --proxy-server=" + interfaceProxy + " --disable-quic",
		Message:     "Codex 已按 " + engine + " 内核模式启动",
	}, nil
}

func mergeEnvironment(base []string, replacements map[string]string) []string {
	keys := make(map[string]struct{}, len(replacements))
	for key := range replacements {
		keys[key] = struct{}{}
	}
	out := make([]string, 0, len(base)+len(replacements))
	for _, item := range base {
		key := item
		if index := indexByte(item, '='); index >= 0 {
			key = item[:index]
		}
		if _, replace := keys[toUpperASCII(key)]; replace {
			continue
		}
		out = append(out, item)
	}
	for key, value := range replacements {
		out = append(out, key+"="+value)
	}
	return out
}

func indexByte(value string, target byte) int {
	for i := 0; i < len(value); i++ {
		if value[i] == target {
			return i
		}
	}
	return -1
}

func toUpperASCII(value string) string {
	data := []byte(value)
	for i, ch := range data {
		if ch >= 'a' && ch <= 'z' {
			data[i] = ch - 32
		}
	}
	return string(data)
}

func FindNativeCodex(ctx context.Context) string {
	result, err := platform.Run(ctx, "cmd.exe", "/c", "where", "codex")
	if err != nil {
		return ""
	}
	for _, line := range splitLines(result.Stdout) {
		if line != "" {
			if stat, statErr := os.Stat(line); statErr == nil && !stat.IsDir() {
				return line
			}
		}
	}
	return ""
}

func CodexVersion(ctx context.Context, executable string) string {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result, err := platform.Run(ctx, executable, "--version")
	if err != nil {
		return ""
	}
	return trimSpace(result.Stdout)
}

func splitLines(value string) []string {
	lines := make([]string, 0)
	start := 0
	for i := 0; i < len(value); i++ {
		if value[i] == '\n' || value[i] == '\r' {
			if i > start {
				lines = append(lines, trimSpace(value[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(value) {
		lines = append(lines, trimSpace(value[start:]))
	}
	return lines
}

func trimSpace(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[0] == '\r' || value[0] == '\n') {
		value = value[1:]
	}
	for len(value) > 0 {
		last := value[len(value)-1]
		if last != ' ' && last != '\t' && last != '\r' && last != '\n' {
			break
		}
		value = value[:len(value)-1]
	}
	return value
}

func FileExists(path string) bool {
	stat, err := os.Stat(path)
	return err == nil && !stat.IsDir()
}

func CleanPath(path string) string {
	if path == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	if abs, err := filepath.Abs(cleaned); err == nil {
		return abs
	}
	return cleaned
}

func PIDString(pid int) string { return strconv.Itoa(pid) }
