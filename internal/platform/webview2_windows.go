//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	messageBoxYesNo         = 0x00000004
	messageBoxIconError     = 0x00000010
	messageBoxIconWarning   = 0x00000030
	messageBoxSetForeground = 0x00010000
	messageBoxYes           = 6
)

var messageBoxW = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

func CheckWebView2() WebView2Status {
	version, err := webView2RegistryVersion()
	if validWebView2Version(version) {
		return WebView2Status{Installed: true, Version: strings.TrimSpace(version)}
	}
	if version, found := webView2InstallDirectoryVersion(); found {
		return WebView2Status{Installed: true, Version: version}
	}
	status := WebView2Status{}
	if err != nil {
		status.Error = err.Error()
	}
	return status
}

func EnsureWebView2() error {
	status := CheckWebView2()
	if status.Installed {
		return nil
	}
	choice := showMessageBox(
		"Codex Lane 需要 WebView2",
		"Codex Lane 的界面依赖 Microsoft Edge WebView2 Runtime，但当前系统没有检测到。\n\n"+
			"是否现在自动下载并安装？安装完成后会自动继续启动。",
		messageBoxYesNo|messageBoxIconWarning|messageBoxSetForeground,
	)
	if choice != messageBoxYes {
		return errors.New("WebView2 Runtime is required")
	}
	if err := InstallWebView2(context.Background()); err != nil {
		showMessageBox(
			"WebView2 安装失败",
			"自动安装没有完成：\n\n"+err.Error()+"\n\n将打开微软官方下载页，请安装后重新启动 Codex Lane。",
			messageBoxIconError|messageBoxSetForeground,
		)
		_ = OpenURL(webView2DownloadPage)
		return err
	}
	for attempt := 0; attempt < 5; attempt++ {
		if status = CheckWebView2(); status.Installed {
			return nil
		}
		time.Sleep(time.Second)
	}
	showMessageBox(
		"WebView2 仍未就绪",
		"安装程序已经结束，但系统仍未报告 WebView2 Runtime。\n\n请重启 Windows 后再试，或从微软官方页面手动安装。",
		messageBoxIconError|messageBoxSetForeground,
	)
	_ = OpenURL(webView2DownloadPage)
	return errors.New("WebView2 Runtime is still unavailable after installation")
}

func InstallWebView2(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 6*time.Minute)
	defer cancel()

	installer := filepath.Join(os.TempDir(), "MicrosoftEdgeWebview2Setup.exe")
	defer os.Remove(installer)
	script := fmt.Sprintf(
		"$ProgressPreference='SilentlyContinue'; [Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12; Invoke-WebRequest -UseBasicParsing -TimeoutSec 300 -Uri '%s' -OutFile '%s'",
		webView2DownloadURL,
		strings.ReplaceAll(installer, "'", "''"),
	)
	if result, err := PowerShell(ctx, script); err != nil {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("下载安装程序失败：%s", message)
	}
	info, err := os.Stat(installer)
	if err != nil {
		return fmt.Errorf("下载安装程序失败：%w", err)
	}
	if info.Size() < 100_000 {
		return errors.New("下载的 WebView2 安装程序不完整")
	}
	result, err := Run(ctx, installer, "/silent", "/install")
	if err != nil {
		message := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("WebView2 安装程序返回错误：%s", message)
	}
	return nil
}

func webView2RegistryVersion() (string, error) {
	paths := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID},
		{registry.CURRENT_USER, `Software\Microsoft\EdgeUpdate\Clients\` + webView2ClientGUID},
	}
	var lastErr error
	for _, item := range paths {
		key, err := registry.OpenKey(item.root, item.path, registry.QUERY_VALUE)
		if err != nil {
			if !errors.Is(err, registry.ErrNotExist) {
				lastErr = err
			}
			continue
		}
		version, _, err := key.GetStringValue("pv")
		_ = key.Close()
		if err == nil && validWebView2Version(version) {
			return version, nil
		}
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			lastErr = err
		}
	}
	return "", lastErr
}

func webView2InstallDirectoryVersion() (string, bool) {
	bases := []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")}
	roots := make([]string, 0, len(bases))
	for _, base := range bases {
		if strings.TrimSpace(base) != "" {
			roots = append(roots, filepath.Join(base, "Microsoft", "EdgeWebView", "Application"))
		}
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for index := len(entries) - 1; index >= 0; index-- {
			entry := entries[index]
			if !entry.IsDir() || !validWebView2Version(entry.Name()) {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, entry.Name(), "msedgewebview2.exe")); err == nil {
				return entry.Name(), true
			}
		}
	}
	return "", false
}

func showMessageBox(title, text string, flags uint32) int {
	titlePtr, _ := windows.UTF16PtrFromString(title)
	textPtr, _ := windows.UTF16PtrFromString(text)
	result, _, _ := messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(flags),
	)
	return int(result)
}
