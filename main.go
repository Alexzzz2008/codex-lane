package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/appservice"
	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/install"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
	"github.com/Alexzzz2008/codex-lane/internal/selfupdate"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var trayIcon []byte

func main() {
	platform.EnsureSelfVisible()
	if len(os.Args) == 4 && os.Args[1] == "--update-guard" {
		pid, err := strconv.Atoi(os.Args[3])
		if err == nil {
			err = selfupdate.GuardUpdate(os.Args[2], pid, 75*time.Second)
		}
		if err != nil {
			platform.AppLog("更新守护：%v", err)
		}
		return
	}
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
	executable, _ := selfupdate.Executable()
	if selfupdate.PredecessorPID() > 0 {
		if err := selfupdate.ArmRecovery(executable); err != nil {
			platform.ShowNotice("Codex Lane", "无法准备更新恢复："+err.Error())
			return
		}
	}
	if err := platform.EnsureWebView2(); err != nil {
		os.Exit(1)
	}
	if install.AutoMigrate() {
		// The installed copy took over; the portable file is removed by it.
		return
	}
	predecessor := selfupdate.PredecessorPID()
	platform.AppLog("启动 v%s exe=%s replaces=%d", appservice.Version, executable, predecessor)
	selfupdate.ReportStarted()
	selfupdate.WaitForPredecessor()
	if predecessor > 0 {
		// The window can only appear once the previous process released the
		// Wails instance mutex. If it never does, put the old build back so the
		// user is not left without a working window.
		if err := selfupdate.WaitInstanceFree("wails-app-"+platform.InstanceID()+"-sim", 25*time.Second); err != nil {
			platform.AppLog("旧实例未退出，尝试回滚：%v", err)
			return
		}
	}
	notifyStaleInstance()

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
			UniqueID: platform.InstanceID(),
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if window != nil {
					window.Show()
					window.Focus()
				}
			},
		},
	})
	if predecessor == 0 {
		if err := selfupdate.ArmRecovery(executable); err != nil {
			platform.ShowNotice("Codex Lane", "无法准备更新恢复："+err.Error())
			return
		}
	}
	service.SetEmitter(func(name string, data ...any) {
		app.Event.Emit(name, data...)
	})
	service.SetQuit(func() {
		platform.AppLog("收到退出请求")
		app.Quit()
	})
	service.SetShowWindow(func() {
		if window != nil {
			window.Show()
			window.Focus()
		}
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
	installSystemTray(app, window, service)

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		fitWindow(app, window)
		platform.AppLog("窗口已显示")
		go install.RemovePending()
		if executable != "" {
			selfupdate.CleanupOld(executable)
		}
	})

	if err := app.Run(); err != nil {
		platform.AppLog("app.Run 返回错误：%v", err)
		log.Fatal(err)
	}
	platform.AppLog("应用已退出")
	platform.ClearInstance()
}

// notifyStaleInstance tells the user why a freshly downloaded build did not
// appear: Windows allows only one Codex Lane window per session, so the running
// process keeps the window and silently swallows the launch.
func notifyStaleInstance() {
	record, ok := platform.ReadInstance()
	if ok && record.PID != os.Getpid() && platform.ProcessAlive(record.PID) && looksLikeLane(record.Executable) {
		// Another Codex Lane owns the window; keep its record and explain why
		// this launch will not replace it.
		running := strings.TrimSpace(record.Version)
		switch {
		case running == "":
		case appservice.CompareVersions(appservice.Version, running) > 0:
			platform.ShowNotice("Codex Lane", fmt.Sprintf(
				"检测到旧版本 v%s 仍在运行，本次启动的 v%s 无法接管窗口。\n\n"+
					"请右键任务栏托盘中的 Codex Lane 图标，选择「退出 Codex Lane」，\n"+
					"再重新打开即可使用 v%s。",
				running, appservice.Version, appservice.Version))
		case appservice.CompareVersions(appservice.Version, running) < 0:
			platform.ShowNotice("Codex Lane", fmt.Sprintf(
				"已有一个更新版本 v%s 正在运行，本次启动的 v%s 不会接管窗口。\n\n"+
					"点击「确定」后将打开正在运行的窗口。",
				running, appservice.Version))
		}
		return
	}
	if err := platform.WriteInstance(appservice.Version); err != nil {
		log.Printf("write instance record: %v", err)
	}
}

func looksLikeLane(executable string) bool {
	if executable == "" {
		return true
	}
	if current, err := os.Executable(); err == nil && strings.EqualFold(current, executable) {
		return true
	}
	return strings.Contains(strings.ToLower(filepath.Base(executable)), "codexlane")
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

func installSystemTray(app *application.App, window *application.WebviewWindow, service *appservice.AppService) {
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
			result, err := service.LaunchCodex(model.LaunchOptions{})
			if err != nil || result.NeedsRestart || result.NeedsInstall {
				window.Show()
				window.Focus()
			}
		}()
	})
	menu.Add("停止通道").OnClick(func(*application.Context) {
		go service.StopLane()
	})
	menu.AddSeparator()
	menu.Add("退出 Codex Lane").OnClick(func(*application.Context) { app.Quit() })
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
