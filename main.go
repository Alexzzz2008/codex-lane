package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/appservice"
	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
	"github.com/Alexzzz2008/codex-lane/internal/selfupdate"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed design/icon/appicon.png
var trayIcon []byte

func main() {
	if commandLineRequested(os.Args[1:]) {
		platform.PrepareConsole()
		if hasArgument(os.Args[1:], "selftest") {
			os.Exit(runSelfTest(os.Args[1:]))
		}
		if hasArgument(os.Args[1:], "version") {
			fmt.Printf("CodexLane %s\n", appservice.Version)
			return
		}
		if hasArgument(os.Args[1:], "check-webview2") {
			os.Exit(printWebView2Status(os.Args[1:]))
		}
		printCommandLineHelp()
		return
	}
	if err := platform.EnsureWebView2(); err != nil {
		os.Exit(1)
	}
	selfupdate.ReportStarted()
	selfupdate.WaitForPredecessor()
	if executable, err := selfupdate.Executable(); err == nil {
		selfupdate.CleanupOld(executable)
	}

	minimizeToTray := loadMinimizeToTray()
	service := appservice.New()
	var window *application.WebviewWindow
	app := application.New(application.Options{
		Name:        "Codex Lane",
		Description: "A dedicated process-level proxy launcher and local observability gateway for Codex Desktop",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "io.github.alexzzz2008.codexlane",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if window != nil {
					window.Show()
					window.Focus()
				}
			},
		},
	})
	service.SetEmitter(func(name string, data ...any) {
		app.Event.Emit(name, data...)
	})
	service.SetQuit(func() {
		app.Quit()
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Codex Lane",
		Width:            1360,
		Height:           860,
		MinWidth:         880,
		MinHeight:        560,
		Hidden:           true,
		BackgroundColour: application.NewRGB(244, 244, 246),
		URL:              "/",
		Windows: application.WindowsWindow{
			EventMapping: windowEventMapping(minimizeToTray),
		},
	})

	if minimizeToTray {
		window.OnWindowEvent(events.Windows.WindowClosing, func(*application.WindowEvent) {
			window.Hide()
		})
	}
	installSystemTray(app, window, service, minimizeToTray)

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		fitWindow(app, window)
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func loadMinimizeToTray() bool {
	paths, err := platform.ResolvePaths()
	if err != nil {
		return true
	}
	cfg, err := config.Load(paths.ConfigPath)
	if err != nil {
		return true
	}
	return cfg.Startup.MinimizeToTray
}

func windowEventMapping(minimizeToTray bool) map[events.WindowEventType]events.WindowEventType {
	if !minimizeToTray {
		return nil
	}
	return map[events.WindowEventType]events.WindowEventType{}
}

func installSystemTray(app *application.App, window *application.WebviewWindow, service *appservice.AppService, minimizeToTray bool) {
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("Codex Lane")
	menu := application.NewMenu()
	menu.Add("打开主窗口").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	menu.Add("打开 Codex").OnClick(func(*application.Context) {
		go func() {
			_, _ = service.LaunchCodex(model.LaunchOptions{})
		}()
	})
	menu.Add("停止通道").OnClick(func(*application.Context) {
		go service.StopLane()
	})
	if minimizeToTray {
		menu.AddSeparator()
		menu.Add("退出 Codex Lane").OnClick(func(*application.Context) {
			app.Quit()
		})
	}
	tray.SetMenu(menu)
	tray.Show()
}

func fitWindow(app *application.App, window *application.WebviewWindow) {
	width, height := 1360, 860
	if screen := app.Screen.GetPrimary(); screen != nil {
		if area := screen.WorkArea; area.Width > 0 && area.Height > 0 {
			width = min(width, area.Width-80)
			height = min(height, area.Height-40)
		}
	}
	window.SetMinSize(min(900, width), min(600, height))
	window.SetSize(width, height)
	window.Center()
	window.Show()
}

func commandLineRequested(args []string) bool {
	return hasArgument(args, "selftest") || hasArgument(args, "version") ||
		hasArgument(args, "check-webview2") || hasArgument(args, "help")
}

func hasArgument(args []string, name string) bool {
	for _, arg := range args {
		if arg == "-"+name || arg == "--"+name || strings.HasPrefix(arg, "--"+name+"=") {
			return true
		}
	}
	return false
}

func printCommandLineHelp() {
	fmt.Println("Codex Lane")
	fmt.Println()
	fmt.Println("  CodexLane.exe             打开桌面控制台")
	fmt.Println("  CodexLane.exe --selftest  运行无界面自检，不启动或重启 Codex")
	fmt.Println("  CodexLane.exe --selftest --json")
	fmt.Println("  CodexLane.exe --version   输出版本号")
	fmt.Println("  CodexLane.exe --check-webview2 [--json]  检查 WebView2 Runtime")
}

func printWebView2Status(args []string) int {
	status := platform.CheckWebView2()
	if hasArgument(args, "json") {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"installed": status.Installed,
			"version":   status.Version,
			"error":     status.Error,
		})
	} else if status.Installed {
		fmt.Printf("WebView2 Runtime %s\n", status.Version)
	} else if status.Error != "" {
		fmt.Fprintf(os.Stderr, "WebView2 Runtime 不可用：%s\n", status.Error)
	} else {
		fmt.Fprintln(os.Stderr, "WebView2 Runtime 未安装")
	}
	if status.Installed {
		return 0
	}
	return 1
}

type selfTestOutput struct {
	Version   string           `json:"version"`
	OK        bool             `json:"ok"`
	CheckedAt string           `json:"checkedAt"`
	Report    model.TestReport `json:"report"`
}

func runSelfTest(args []string) int {
	service := appservice.New()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := service.PrepareSelfTest(ctx); err != nil {
		if hasArgument(args, "json") {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"version": appservice.Version,
				"ok":      false,
				"error":   err.Error(),
			})
		} else {
			fmt.Fprintf(os.Stderr, "自检初始化失败：%v\n", err)
		}
		return 2
	}
	defer service.StopLane()

	report, err := service.RunTests()
	if err != nil {
		if hasArgument(args, "json") {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"version": appservice.Version,
				"ok":      false,
				"error":   err.Error(),
			})
		} else {
			fmt.Fprintf(os.Stderr, "自检运行失败：%v\n", err)
		}
		return 2
	}
	output := selfTestOutput{
		Version:   appservice.Version,
		OK:        report.Healthy,
		CheckedAt: report.FinishedAt,
		Report:    report,
	}
	if hasArgument(args, "json") {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(output); err != nil {
			fmt.Fprintf(os.Stderr, "输出 JSON 失败：%v\n", err)
			return 2
		}
	} else {
		printSelfTestText(output)
	}
	if output.OK {
		return 0
	}
	return 1
}

func printSelfTestText(output selfTestOutput) {
	fmt.Printf("Codex Lane 自检 v%s\n", output.Version)
	fmt.Printf("时间：%s\n\n", output.CheckedAt)
	for _, result := range output.Report.Results {
		latency := "-"
		if result.LatencyMs > 0 {
			latency = fmt.Sprintf("%dms", result.LatencyMs)
		}
		fmt.Printf("%-4s  %-5s  %-25s %8s  %s\n", result.ID, result.Status, result.Name, latency, result.Detail)
		if result.Hint != "" && result.Status != "PASS" && result.Status != "SKIP" {
			fmt.Printf("      提示：%s\n", result.Hint)
		}
	}
	state := "通过"
	if !output.OK {
		state = "存在失败项"
	}
	fmt.Printf("\n结果：%s · %d 分 · %d 项 · %dms\n", state, output.Report.Score, len(output.Report.Results), output.Report.DurationMs)
}
