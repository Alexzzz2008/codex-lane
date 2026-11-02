package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
)

type PackageInfo struct {
	Name            string `json:"Name"`
	Version         string `json:"Version"`
	InstallLocation string `json:"InstallLocation"`
	PackageFullName string `json:"PackageFullName"`
	ExecutablePath  string `json:"-"`
}

func Locate(ctx context.Context) (PackageInfo, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	script := `
$package = Get-AppxPackage -Name OpenAI.Codex | Sort-Object Version -Descending | Select-Object -First 1
if ($null -eq $package) { throw 'OpenAI.Codex is not installed' }
[pscustomobject]@{
  Name = $package.Name
  Version = [string]$package.Version
  InstallLocation = $package.InstallLocation
  PackageFullName = $package.PackageFullName
}`
	var info PackageInfo
	if err := platform.PowerShellJSON(ctx, script, &info); err != nil {
		return info, err
	}
	for _, name := range []string{"ChatGPT.exe", "Codex.exe"} {
		candidate := filepath.Join(info.InstallLocation, "app", name)
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			info.ExecutablePath = candidate
			return info, nil
		}
	}
	return info, errors.New("Codex package found, but app\\ChatGPT.exe or app\\Codex.exe is missing")
}

func (p PackageInfo) Status() model.CodexStatus {
	return model.CodexStatus{
		Installed:       p.ExecutablePath != "",
		PackageName:     p.Name,
		Version:         p.Version,
		InstallLocation: p.InstallLocation,
		ExecutablePath:  p.ExecutablePath,
	}
}

type rawProcess struct {
	ProcessID       uint32 `json:"ProcessId"`
	ParentProcessID uint32 `json:"ParentProcessId"`
	Name            string `json:"Name"`
	ExecutablePath  string `json:"ExecutablePath"`
	CommandLine     string `json:"CommandLine"`
}

func ListProcesses(ctx context.Context, installLocation string) ([]model.Process, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	script := `
$items = @(Get-CimInstance Win32_Process -Filter "Name='ChatGPT.exe' OR Name='Codex.exe'" | Where-Object {
  $_.ExecutablePath -and
  $_.ExecutablePath.StartsWith('{{INSTALL}}', [System.StringComparison]::OrdinalIgnoreCase)
} | Select-Object ProcessId,ParentProcessId,Name,ExecutablePath,CommandLine)
[pscustomobject]@{ Items = $items }`
	script = strings.ReplaceAll(script, "{{INSTALL}}", strings.ReplaceAll(installLocation, "'", "''"))
	var payload struct {
		Items []rawProcess `json:"Items"`
	}
	if err := platform.PowerShellJSON(ctx, script, &payload); err != nil {
		return nil, err
	}
	raw := payload.Items
	out := make([]model.Process, 0, len(raw))
	for _, process := range raw {
		main := process.CommandLine != "" && !strings.Contains(strings.ToLower(process.CommandLine), "--type=")
		out = append(out, model.Process{
			PID:         int(process.ProcessID),
			ParentPID:   int(process.ParentProcessID),
			Name:        process.Name,
			Executable:  process.ExecutablePath,
			CommandLine: process.CommandLine,
			IsMain:      main,
		})
	}
	return out, nil
}

func ProxyServer(commandLine string) string {
	const flag = "--proxy-server="
	lower := strings.ToLower(commandLine)
	index := strings.Index(lower, flag)
	if index < 0 {
		return ""
	}
	value := commandLine[index+len(flag):]
	if space := strings.IndexAny(value, " \t\r\n"); space >= 0 {
		value = value[:space]
	}
	value = strings.Trim(value, `"'`)
	return strings.TrimSuffix(value, "/")
}

func StatusWithProcesses(info PackageInfo, processes []model.Process) model.CodexStatus {
	status := info.Status()
	status.Processes = processes
	for _, process := range processes {
		if !process.IsMain {
			continue
		}
		status.Running = true
		status.MainPID = process.PID
		status.CommandLine = process.CommandLine
		status.ProxyURL = ProxyServer(process.CommandLine)
		status.ProxyMatched = status.ProxyURL != ""
		break
	}
	return status
}

func RawJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func ValidateCodexPath(executable, installLocation string) error {
	absExe, err := filepath.Abs(executable)
	if err != nil {
		return err
	}
	absInstall, err := filepath.Abs(installLocation)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absInstall, absExe)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("Codex executable is outside the trusted package directory")
	}
	return nil
}
