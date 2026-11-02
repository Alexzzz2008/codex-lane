package platform

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

type windowsEnvironment struct {
	Caption      string `json:"Caption"`
	Version      string `json:"Version"`
	Build        string `json:"Build"`
	ProxyEnabled int    `json:"ProxyEnabled"`
	ProxyServer  string `json:"ProxyServer"`
	Admin        bool   `json:"Admin"`
}

func InspectEnvironment(ctx context.Context, paths Paths) model.Environment {
	environment := model.Environment{
		OSName:        runtime.GOOS,
		Arch:          runtime.GOARCH,
		UserHome:      paths.UserHome,
		CodexHome:     paths.CodexHome,
		ConfigPath:    paths.ConfigPath,
		LauncherPath:  paths.ConfigPath,
		LogDirectory:  paths.LogDirectory,
		GoroutineSafe: true,
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	script := `
$os = Get-CimInstance Win32_OperatingSystem
$internet = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -ErrorAction SilentlyContinue
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
[pscustomobject]@{
  Caption = $os.Caption
  Version = $os.Version
  Build = $os.BuildNumber
  ProxyEnabled = if ($null -ne $internet.ProxyEnable) { [int]$internet.ProxyEnable } else { 0 }
  ProxyServer = if ($null -ne $internet.ProxyServer) { [string]$internet.ProxyServer } else { '' }
  Admin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}`
	var info windowsEnvironment
	if err := PowerShellJSON(ctx, script, &info); err != nil {
		environment.OSName = "Windows"
		environment.OSVersion = "无法读取"
		return environment
	}
	environment.OSName = firstNonEmpty(info.Caption, "Windows")
	environment.OSVersion = strings.TrimSpace(info.Version + " · " + info.Build)
	environment.SystemProxyOn = info.ProxyEnabled != 0
	environment.SystemProxy = info.ProxyServer
	environment.Administrator = info.Admin
	return environment
}

func CheckSystemProxy(ctx context.Context) (bool, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var info windowsEnvironment
	if err := PowerShellJSON(ctx, `
$internet = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -ErrorAction SilentlyContinue
[pscustomobject]@{
  ProxyEnabled = if ($null -ne $internet.ProxyEnable) { [int]$internet.ProxyEnable } else { 0 }
  ProxyServer = if ($null -ne $internet.ProxyServer) { [string]$internet.ProxyServer } else { '' }
}`, &info); err != nil {
		return false, "", err
	}
	return info.ProxyEnabled != 0, info.ProxyServer, nil
}

func IsAdministrator(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	var info windowsEnvironment
	if err := PowerShellJSON(ctx, `
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
[pscustomobject]@{ Admin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) }`, &info); err != nil {
		return false
	}
	return info.Admin
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func DescribeOS() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}
