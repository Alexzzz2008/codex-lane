package appservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Alexzzz2008/codex-lane/internal/codex"
	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/gateway"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/observe"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
	"github.com/Alexzzz2008/codex-lane/internal/preflight"
	"github.com/Alexzzz2008/codex-lane/internal/selfupdate"
)

const Version = "0.1.33"

type Emitter func(name string, data ...any)

type AppService struct {
	mu                    sync.RWMutex
	ctx                   context.Context
	paths                 platform.Paths
	cfg                   model.Config
	store                 *observe.Store
	lane                  *gateway.Server
	laneCtx               context.Context
	laneCancel            context.CancelFunc
	emitFn                Emitter
	events                []model.Event
	initError             string
	testReport            *model.TestReport
	diagnostic            *model.DiagnosticReport
	lastDeep              time.Time
	lastCodexCheck        time.Time
	cachedPackage         codex.PackageInfo
	cachedCodex           model.CodexStatus
	cachedDeps            []model.Dependency
	cachedWSL             model.WSLStatus
	cachedEngine          model.ResolvedEngine
	cachedEnvironment     model.Environment
	cachedUpstream        model.UpstreamStatus
	updateStatus          model.UpdateStatus
	updateManifest        selfupdate.Manifest
	lastProbe             time.Time
	testRunning           atomic.Bool
	updateCheckRunning    atomic.Bool
	updateDownloadRunning atomic.Bool
	updateInstallRunning  atomic.Bool
	quitFn                func()
	stagedUpdate          string
	stagedVersion         string
	selfTest              bool
}

func New() *AppService {
	return &AppService{
		updateStatus: model.UpdateStatus{
			CurrentVersion: Version,
			Status:         "unknown",
		},
	}
}

//wails:ignore
func (s *AppService) SetEmitter(emitter Emitter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitFn = emitter
}

func (s *AppService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	return s.startup(ctx)
}

func (s *AppService) startup(ctx context.Context) error {
	paths, err := platform.ResolvePaths()
	if err != nil {
		return err
	}
	cfg, cfgErr := config.Load(paths.ConfigPath)
	if cfgErr != nil {
		cfg = config.Default()
		s.initError = cfgErr.Error()
	}
	s.mu.Lock()
	s.ctx = ctx
	s.paths = paths
	s.cfg = cfg
	s.store = observe.NewStore(cfg.Logging.MaxMemoryRows, paths.LogDirectory, nil)
	s.mu.Unlock()
	s.addEvent("info", "Codex Lane 已启动", "本地控制台已就绪")
	if cfgErr != nil {
		s.addEvent("error", "配置加载失败", cfgErr.Error())
	}
	if err := s.StartLane(); err != nil {
		s.addEvent("error", "本地入口启动失败", err.Error())
	} else {
		s.addEvent("ok", "本地网关已就绪", "Codex 与内核将使用独立双入口")
	}
	s.Refresh()
	s.startMonitor()
	s.startUpdateLoop()
	if cfg.Startup.OpenCodex {
		_, _ = s.LaunchCodex(model.LaunchOptions{})
	}
	return nil
}

func (s *AppService) ServiceShutdown() error {
	s.StopLane()
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store != nil {
		store.Close()
	}
	s.mu.RLock()
	stopCodex := s.cfg.Startup.StopCodexOnExit
	installPath := s.cachedCodex.InstallLocation
	pid := s.cachedCodex.MainPID
	s.mu.RUnlock()
	if stopCodex && pid > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = codex.KillMainProcess(ctx, installPath, pid)
	}
	s.installUpdateOnExit()
	return nil
}

func (s *AppService) Bootstrap() (model.Snapshot, error) {
	return s.Refresh()
}

//wails:ignore
func (s *AppService) PrepareSelfTest(ctx context.Context) error {
	paths, err := platform.ResolvePaths()
	if err != nil {
		return err
	}
	cfg, cfgErr := config.Load(paths.ConfigPath)
	if cfgErr != nil {
		cfg = config.Default()
		s.initError = cfgErr.Error()
	}
	cfg.Lane.InterfacePort = 0
	cfg.Lane.KernelPort = 0
	s.mu.Lock()
	s.ctx = ctx
	s.paths = paths
	s.cfg = cfg
	s.selfTest = true
	s.store = observe.NewStore(cfg.Logging.MaxMemoryRows, paths.LogDirectory, nil)
	s.mu.Unlock()
	return s.StartLane()
}

func (s *AppService) GetState() (model.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return s.snapshot(ctx, false), nil
}

func (s *AppService) Refresh() (model.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return s.snapshot(ctx, true), nil
}

func (s *AppService) SaveSettings(input model.SettingsInput) (model.Snapshot, error) {
	s.mu.Lock()
	cfg := s.cfg
	cfgPath := s.paths.ConfigPath
	codexConfig := filepath.Join(s.paths.CodexHome, "config.toml")
	previousEngine := s.cfg.Engine
	previousDistro := s.cfg.WSLDistro
	previousRoutes := s.cfg.Lane.Routes
	previousAllowed := s.cfg.Lane.AllowedHosts
	previousFilter := s.cfg.Lane.HostFilterMode
	previousAutoStart := platform.IsAutoStart()
	s.mu.Unlock()

	if err := config.ApplySettings(&cfg, input); err != nil {
		return model.Snapshot{}, err
	}
	engine, err := codex.ResolveEngine(cfg, codexConfig)
	if err != nil && cfg.Engine != model.EngineAuto {
		return model.Snapshot{}, err
	}
	if cfg.Engine != model.EngineAuto {
		targetWSL := cfg.Engine == model.EngineWSL
		if engine.KeyPresent && engine.KeyValue != targetWSL {
			if _, err := codex.SetEngine(codexConfig, targetWSL); err != nil {
				return model.Snapshot{}, fmt.Errorf("switch Codex engine: %w", err)
			}
			s.addEvent("info", "内核环境已切换", "写入前已备份 config.toml")
		}
	}
	oldUpstream := config.UpstreamURL(s.currentConfig().Upstream)
	if err := config.Save(cfgPath, cfg); err != nil {
		return model.Snapshot{}, err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	if input.LaunchAtLogin != previousAutoStart {
		if err := platform.SetAutoStart(input.LaunchAtLogin); err != nil {
			s.addEvent("error", "开机自启设置失败", err.Error())
		}
	}
	newUpstream := config.UpstreamURL(cfg.Upstream)
	laneRulesChanged := !sameRoutes(previousRoutes, cfg.Lane.Routes) ||
		!sameStrings(previousAllowed, cfg.Lane.AllowedHosts) ||
		previousFilter != cfg.Lane.HostFilterMode
	if oldUpstream != newUpstream || laneRulesChanged {
		if err := s.restartLane(); err != nil {
			s.addEvent("error", "网关重启失败", err.Error())
		} else {
			s.addEvent("ok", "上游代理已更新", newUpstream)
		}
	}
	s.addEvent("info", "设置已保存", config.UpstreamURL(cfg.Upstream))
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer refreshCancel()
	engineChanged := previousEngine != cfg.Engine || previousDistro != cfg.WSLDistro
	return s.snapshotWith(refreshCtx, engineChanged, false), nil
}

func sameRoutes(a, b []model.RouteRule) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func (s *AppService) StartLane() error {
	s.mu.Lock()
	if s.lane != nil {
		s.mu.Unlock()
		return nil
	}
	if s.ctx == nil {
		s.mu.Unlock()
		return errors.New("app service is not ready")
	}
	cfg := s.cfg
	s.mu.Unlock()
	return s.startLane(cfg)
}

func (s *AppService) startLane(cfg model.Config) error {
	s.mu.Lock()
	if s.lane != nil {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	if cfg.RequireHealthyProxy {
		address := fmt.Sprintf("%s:%d", cfg.Upstream.Host, cfg.Upstream.Port)
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 4*time.Second)
		_, probeErr := gateway.CheckTCPService(checkCtx, address)
		checkCancel()
		if probeErr != nil {
			return fmt.Errorf("上游代理不可达（%s）：%w", address, probeErr)
		}
	}
	ctx, cancel := context.WithCancel(s.ctx)
	lane, err := gateway.Start(ctx, gateway.Options{
		Upstream:            cfg.Upstream,
		Host:                cfg.Lane.ListenHost,
		PreferredPort:       cfg.Lane.InterfacePort,
		PreferredKernelPort: cfg.Lane.KernelPort,
		Routes:              cfg.Lane.Routes,
		AllowedHosts:        cfg.Lane.AllowedHosts,
		HostFilterMode:      cfg.Lane.HostFilterMode,
		OnRecord:            s.handleGatewayEvent,
	})
	if err != nil {
		cancel()
		return err
	}
	s.mu.Lock()
	s.lane = lane
	s.laneCtx = ctx
	s.laneCancel = cancel
	s.mu.Unlock()
	s.persistLanePorts(lane)
	return nil
}

func (s *AppService) persistLanePorts(lane *gateway.Server) {
	s.mu.Lock()
	if s.selfTest {
		s.mu.Unlock()
		return
	}
	if s.cfg.Lane.InterfacePort == lane.InterfacePort() && s.cfg.Lane.KernelPort == lane.KernelPort() {
		s.mu.Unlock()
		return
	}
	s.cfg.Lane.InterfacePort = lane.InterfacePort()
	s.cfg.Lane.KernelPort = lane.KernelPort()
	cfg := s.cfg
	path := s.paths.ConfigPath
	s.mu.Unlock()
	if err := config.Save(path, cfg); err != nil {
		s.addEvent("error", "端口记忆失败", err.Error())
	}
}

func (s *AppService) StopLane() {
	s.mu.Lock()
	lane := s.lane
	cancel := s.laneCancel
	s.lane = nil
	s.laneCtx = nil
	s.laneCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if lane != nil {
		ctx, cancelTimeout := context.WithTimeout(context.Background(), 3*time.Second)
		_ = lane.Close(ctx)
		cancelTimeout()
		s.addEvent("info", "本地网关已停止", "Codex 现有连接可能随之中断")
	}
}

func (s *AppService) restartLane() error {
	s.StopLane()
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	return s.startLane(cfg)
}

func (s *AppService) LaunchCodex(options model.LaunchOptions) (model.LaunchResult, error) {
	if err := s.StartLane(); err != nil {
		return model.LaunchResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg := s.currentConfig()
	paths := s.currentPaths()
	engine, err := codex.ResolveEngine(cfg, filepath.Join(paths.CodexHome, "config.toml"))
	if err != nil {
		return model.LaunchResult{}, err
	}
	if engine.Resolved == model.EngineWSL {
		wsl := preflight.Inspect(ctx, paths.UserHome, paths.CodexHome, cfg.WSLDistro)
		if err := preflight.ValidateMode(wsl); err != nil {
			return model.LaunchResult{}, err
		}
	}
	pkg, err := codex.Locate(ctx)
	if err != nil {
		return model.LaunchResult{
			NeedsInstall: true,
			Message:      "未检测到 Codex Desktop：" + err.Error(),
		}, nil
	}
	processes, err := codex.ListProcesses(ctx, pkg.InstallLocation)
	if err != nil {
		return model.LaunchResult{}, err
	}
	status := codex.StatusWithProcesses(pkg, processes)
	lane := s.currentLane()
	if lane == nil {
		return model.LaunchResult{}, gateway.ErrNoLane
	}
	if status.Running {
		connected := strings.EqualFold(status.ProxyURL, lane.InterfaceURL())
		if connected && !options.RestartExisting {
			return model.LaunchResult{
				AlreadyRunning: true,
				PID:            status.MainPID,
				Message:        "Codex 已在 Lane 通道中运行",
			}, nil
		}
		if !options.RestartExisting {
			return model.LaunchResult{
				NeedsRestart: true,
				PID:          status.MainPID,
				Message:      "Codex 正在运行，当前代理入口为 " + proxyLabel(status.ProxyURL),
			}, nil
		}
		if err := codex.KillMainProcess(ctx, pkg.InstallLocation, status.MainPID); err != nil {
			return model.LaunchResult{}, err
		}
		s.mu.Lock()
		s.lastCodexCheck = time.Time{}
		s.mu.Unlock()
		s.addEvent("info", "Codex 已停止", "等待重新注入代理参数")
	}
	if cfg.RequireHealthyProxy {
		probeCtx, probeCancel := context.WithTimeout(ctx, 10*time.Second)
		_, probeErr := gateway.CheckUpstreamTunnel(probeCtx, cfg.Upstream, "auth.openai.com:443")
		probeCancel()
		if probeErr != nil {
			return model.LaunchResult{}, fmt.Errorf("上游代理无法出网，请先切换 Clash 节点后重试：%w", probeErr)
		}
	}
	result, err := codex.Launch(pkg, engine.Resolved, lane.InterfaceURL(), lane.KernelURL(), cfg.Upstream.NoProxy, paths.UserHome)
	if err != nil {
		s.addEvent("error", "Codex 启动失败", err.Error())
		return result, err
	}
	s.addEvent("ok", "Codex 已启动", "PID "+fmt.Sprint(result.PID)+" · "+engine.Resolved)
	s.mu.Lock()
	s.lastCodexCheck = time.Time{}
	s.mu.Unlock()
	return result, nil
}

func proxyLabel(url string) string {
	value := strings.TrimSpace(url)
	if value == "" {
		return "未设置代理"
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{"http://", "https://", "socks5://", "socks5h://"} {
		if strings.HasPrefix(lower, prefix) {
			return value[len(prefix):]
		}
	}
	return value
}

func (s *AppService) StopCodex() (model.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pkg, err := codex.Locate(ctx)
	if err != nil {
		return model.Snapshot{}, err
	}
	processes, err := codex.ListProcesses(ctx, pkg.InstallLocation)
	if err != nil {
		return model.Snapshot{}, err
	}
	status := codex.StatusWithProcesses(pkg, processes)
	if !status.Running {
		return s.Refresh()
	}
	if err := codex.KillMainProcess(ctx, pkg.InstallLocation, status.MainPID); err != nil {
		return model.Snapshot{}, err
	}
	s.mu.Lock()
	s.lastCodexCheck = time.Time{}
	s.mu.Unlock()
	s.addEvent("info", "Codex 已停止", "未修改系统网络设置")
	return s.Refresh()
}

func (s *AppService) ProbeProxy(input model.SettingsInput) model.UpstreamStatus {
	cfg := s.currentConfig()
	cfg.Upstream.Protocol = strings.ToLower(strings.TrimSpace(input.Protocol))
	cfg.Upstream.Host = strings.TrimSpace(input.Host)
	cfg.Upstream.Port = input.Port
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.probeUpstream(ctx, cfg)
}

func (s *AppService) RunDiagnostics() (model.DiagnosticReport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	report := s.diagnose(ctx)
	s.mu.Lock()
	s.diagnostic = &report
	s.mu.Unlock()
	return report, nil
}

func (s *AppService) RunTests() (model.TestReport, error) {
	if !s.testRunning.CompareAndSwap(false, true) {
		return model.TestReport{}, errors.New("检查正在进行中，请稍候")
	}
	defer s.testRunning.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	report := s.runTests(ctx)
	s.mu.Lock()
	s.testReport = &report
	s.mu.Unlock()
	return report, nil
}

func (s *AppService) ClearRequests() (model.Snapshot, error) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store != nil {
		store.Clear()
	}
	return s.Refresh()
}

func (s *AppService) ExportReport() (string, error) {
	snapshot, err := s.Refresh()
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *AppService) OpenLogsFolder() error {
	paths := s.currentPaths()
	if err := os.MkdirAll(paths.LogDirectory, 0o700); err != nil {
		return err
	}
	return platform.OpenURL(paths.LogDirectory)
}

func (s *AppService) OpenConfigFolder() error {
	paths := s.currentPaths()
	dir := filepath.Dir(paths.ConfigPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return platform.OpenURL(dir)
}

func (s *AppService) OpenProjectPage(target string) error {
	switch target {
	case "repository":
		return platform.OpenURL(RepositoryURL)
	case "releases":
		return platform.OpenURL(ReleasesURL)
	default:
		return errors.New("unsupported project page")
	}
}

func (s *AppService) SetLaunchAtLogin(enable bool) error {
	return platform.SetAutoStart(enable)
}

func (s *AppService) currentConfig() model.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *AppService) currentPaths() platform.Paths {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paths
}

func (s *AppService) currentLane() *gateway.Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lane
}

func (s *AppService) handleGatewayEvent(event string, record model.RequestRecord) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store != nil {
		switch event {
		case "request:new":
			store.Add(record)
		case "request:update":
			store.Touch(record)
		case "request:finished":
			store.Finish(record)
		}
	}
	s.emit("lane:request", map[string]any{
		"event":   event,
		"request": record,
	})
}

func (s *AppService) addEvent(kind, title, detail string) {
	event := model.Event{
		Time:   time.Now().Format("15:04"),
		Kind:   kind,
		Title:  title,
		Detail: detail,
	}
	s.mu.Lock()
	s.events = append([]model.Event{event}, s.events...)
	if len(s.events) > 30 {
		s.events = s.events[:30]
	}
	s.mu.Unlock()
	s.emit("lane:event", event)
}

func (s *AppService) emit(name string, data ...any) {
	s.mu.RLock()
	emitter := s.emitFn
	s.mu.RUnlock()
	if emitter != nil {
		emitter(name, data...)
	}
}

func (s *AppService) requestLogPath() string {
	paths := s.currentPaths()
	entries, err := os.ReadDir(paths.LogDirectory)
	if err != nil {
		return paths.LogDirectory
	}
	latest := ""
	var latestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "requests-") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr == nil && info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = filepath.Join(paths.LogDirectory, entry.Name())
		}
	}
	if latest == "" {
		return paths.LogDirectory
	}
	return latest
}
