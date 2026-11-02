package appservice

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/codex"
	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/gateway"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
	"github.com/Alexzzz2008/codex-lane/internal/preflight"
)

func (s *AppService) checkDependencies(ctx context.Context) []model.Dependency {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cfg := s.currentConfig()
	paths := s.currentPaths()
	deps := make([]model.Dependency, 0, 4)

	webViewDep := model.Dependency{
		ID: "webview2", Name: "Microsoft Edge WebView2", Required: true, Action: "install-webview2",
	}
	webView := platform.CheckWebView2()
	if webView.Installed {
		webViewDep.Ready = true
		webViewDep.Detail = "Evergreen Runtime " + webView.Version
	} else {
		webViewDep.Detail = "未检测到 Evergreen Runtime"
		webViewDep.Hint = "界面依赖；也可以在启动前由原生提示自动安装"
		if webView.Error != "" {
			webViewDep.Hint = webView.Error
		}
	}
	deps = append(deps, webViewDep)

	codexDep := model.Dependency{
		ID: "codex", Name: "Codex Desktop", Required: true, Action: "install-codex",
	}
	if pkg, err := codex.Locate(ctx); err != nil {
		codexDep.Detail = "未检测到 OpenAI.Codex 安装包"
		codexDep.Hint = "可通过 winget 安装，失败时会打开官方下载页"
	} else {
		codexDep.Ready = true
		codexDep.Detail = "已安装 " + pkg.Version
	}
	deps = append(deps, codexDep)

	engine, _ := codex.ResolveEngine(cfg, filepath.Join(paths.CodexHome, "config.toml"))
	if engine.Resolved == model.EngineWSL {
		wslDep := model.Dependency{
			ID: "wsl", Name: "WSL 2 运行环境", Required: true, Action: "install-wsl",
		}
		wsl := preflight.Inspect(ctx, paths.UserHome, paths.CodexHome, cfg.WSLDistro)
		switch {
		case wsl.Distro == "":
			wslDep.Detail = "未检测到可用的 WSL 发行版"
			wslDep.Hint = "安装需要管理员权限，完成后可能需要重启"
		case wsl.NetworkMode != "mirrored":
			wslDep.Detail = wsl.Distro + " · " + wsl.NetworkMode
			wslDep.Hint = "Lane 需要 mirrored 网络模式"
		default:
			wslDep.Ready = true
			wslDep.Detail = wsl.Distro + " · mirrored"
		}
		deps = append(deps, wslDep)
	}

	upstreamDep := model.Dependency{ID: "upstream", Name: "上游代理", Required: true}
	address := fmt.Sprintf("%s:%d", cfg.Upstream.Host, cfg.Upstream.Port)
	if latency, err := gateway.CheckTCPService(ctx, address); err != nil {
		upstreamDep.Detail = address + " 无法连接"
		upstreamDep.Hint = "先启动 Clash / Mihomo 等代理软件"
	} else {
		probeCtx, probeCancel := context.WithTimeout(ctx, 6*time.Second)
		_, _, _, probeErr := gateway.HTTPBodyThroughProxy(probeCtx, config.UpstreamURL(cfg.Upstream), authDiscoveryURL)
		probeCancel()
		if probeErr != nil {
			upstreamDep.Detail = fmt.Sprintf("%s · 端口可连（%d ms），但无法出网", address, latency.Milliseconds())
			upstreamDep.Hint = "切换 Clash 节点，或检查规则是否把 OpenAI 指到了不可用节点"
		} else {
			upstreamDep.Ready = true
			upstreamDep.Detail = fmt.Sprintf("%s · %d ms · 出网正常", address, latency.Milliseconds())
		}
	}
	deps = append(deps, upstreamDep)

	return deps
}

func (s *AppService) GetDependencies() ([]model.Dependency, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	deps := s.checkDependencies(ctx)
	s.mu.Lock()
	s.cachedDeps = deps
	s.mu.Unlock()
	return deps, nil
}

func (s *AppService) InstallDependency(id string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "codex":
		return s.installCodex()
	case "wsl":
		return s.installWSL()
	case "webview2":
		if err := platform.InstallWebView2(context.Background()); err != nil {
			return "", err
		}
		status := platform.CheckWebView2()
		if !status.Installed {
			return "安装程序已结束，但暂时没有检测到 WebView2 Runtime；请重启后再确认", nil
		}
		return "WebView2 Runtime 安装完成：" + status.Version, nil
	default:
		return "", errors.New("unsupported dependency: " + id)
	}
}

func (s *AppService) installCodex() (string, error) {
	if _, err := exec.LookPath("winget"); err != nil {
		_ = platform.OpenURL("https://chatgpt.com/download")
		return "未找到 winget，已打开官方下载页", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	result, err := platform.Run(ctx, "winget", "install", "--id", "OpenAI.Codex",
		"--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity")
	if err != nil {
		_ = platform.OpenURL("https://chatgpt.com/download")
		message := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
		if message == "" {
			message = err.Error()
		}
		return "winget 安装未完成，已打开官方下载页\n" + trimForDisplay(message, 400), nil
	}
	s.mu.Lock()
	s.lastCodexCheck = time.Time{}
	s.mu.Unlock()
	return "Codex Desktop 安装完成", nil
}

func (s *AppService) installWSL() (string, error) {
	script := "Start-Process -FilePath wsl.exe -ArgumentList '--install','-d','Ubuntu' -Verb RunAs"
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := platform.Run(ctx, "powershell.exe", "-NoProfile", "-Command", script); err != nil {
		return "", fmt.Errorf("启动 WSL 安装程序失败：%w", err)
	}
	return "已请求管理员权限安装 WSL；安装完成后按提示重启，再回来重新检查", nil
}
