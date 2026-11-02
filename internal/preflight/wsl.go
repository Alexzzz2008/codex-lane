package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
)

func Inspect(ctx context.Context, home string, codexHome string, preferredDistro string) model.WSLStatus {
	status := model.WSLStatus{Distro: preferredDistro}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	distroResult, err := platform.Run(ctx, "wsl.exe", "-l", "-q")
	if err != nil {
		status.Error = commandError("wsl.exe -l -q", distroResult, err)
		return status
	}
	status.Available = true
	for _, line := range decodeWSLOutput(distroResult.Stdout) {
		line = strings.TrimSpace(line)
		if line != "" {
			status.Distros = append(status.Distros, line)
		}
	}
	if status.Distro == "" && len(status.Distros) > 0 {
		status.Distro = status.Distros[0]
	}
	if status.Distro == "" {
		status.Error = "WSL 未安装任何发行版"
		return status
	}

	interop, interopErr := platform.Run(ctx, "wsl.exe", "-d", status.Distro, "-e", "true")
	status.InteropOK = interopErr == nil && interop.Code == 0
	if !status.InteropOK {
		status.Error = commandError("WSL interop", interop, interopErr)
	}

	network, networkErr := platform.Run(ctx, "wsl.exe", "-d", status.Distro, "-e", "wslinfo", "--networking-mode")
	if networkErr == nil {
		status.NetworkMode = strings.TrimSpace(decodeWSLOutputSingle(network.Stdout))
	}
	if status.NetworkMode == "" {
		status.NetworkMode = "unknown"
	}
	status.KernelPath = FindKernel(codexHome)
	status.KernelFound = status.KernelPath != ""
	if status.Error == "" && !status.KernelFound {
		status.Error = "未找到 WSL Codex 内核，请先启动一次 Codex 桌面端"
	}
	return status
}

func FindKernel(codexHome string) string {
	root := filepath.Join(codexHome, "bin", "wsl")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	type candidate struct {
		path    string
		modTime time.Time
	}
	var candidates []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "codex")
		stat, statErr := os.Stat(path)
		if statErr == nil && !stat.IsDir() {
			candidates = append(candidates, candidate{path: path, modTime: stat.ModTime()})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].modTime.After(candidates[j].modTime) })
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].path
}

func decodeWSLOutput(value string) []string {
	text := decodeWSLOutputSingle(value)
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func decodeWSLOutputSingle(value string) string {
	raw := []byte(value)
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		raw = raw[2:]
	}
	if len(raw)%2 == 0 {
		zeros := 0
		for i := 1; i < len(raw); i += 2 {
			if raw[i] == 0 {
				zeros++
			}
		}
		if len(raw) > 0 && zeros > len(raw)/4 {
			units := make([]uint16, 0, len(raw)/2)
			for i := 0; i+1 < len(raw); i += 2 {
				units = append(units, uint16(raw[i])|uint16(raw[i+1])<<8)
			}
			return string(utf16.Decode(units))
		}
	}
	return strings.ToValidUTF8(string(raw), "")
}

func commandError(name string, result platform.CommandResult, err error) string {
	message := strings.TrimSpace(result.Stderr)
	if message == "" {
		message = strings.TrimSpace(result.Stdout)
	}
	if message == "" && err != nil {
		message = err.Error()
	}
	return fmt.Sprintf("%s 失败：%s", name, message)
}

func ValidateMode(status model.WSLStatus) error {
	if !status.Available {
		return errors.New(status.Error)
	}
	if !status.InteropOK {
		return errors.New("WSL interop 异常，请执行 wsl --shutdown 后重试")
	}
	if !status.KernelFound {
		return errors.New(status.Error)
	}
	if status.NetworkMode != "mirrored" {
		return fmt.Errorf("WSL 网络模式为 %s；首版要求 mirrored，请在 .wslconfig 中切换后执行 wsl --shutdown", status.NetworkMode)
	}
	return nil
}

func ParseDistroList(value string) []string {
	out := make([]string, 0)
	for _, line := range decodeWSLOutput(value) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func IsValidDistro(value string) bool {
	if value == "" {
		return true
	}
	for _, ch := range value {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' && ch != '.' {
			return false
		}
	}
	return true
}

func Int(value int) string { return strconv.Itoa(value) }
