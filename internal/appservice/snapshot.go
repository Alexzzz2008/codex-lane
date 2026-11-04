package appservice

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/codex"
	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/gateway"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
	"github.com/Alexzzz2008/codex-lane/internal/preflight"
)

func (s *AppService) snapshot(ctx context.Context, deep bool) model.Snapshot {
	return s.snapshotWith(ctx, deep, true)
}

func (s *AppService) snapshotWith(ctx context.Context, deep bool, allowProbe bool) model.Snapshot {
	s.mu.RLock()
	cfg := s.cfg
	paths := s.paths
	lane := s.lane
	store := s.store
	cachedCodex := s.cachedCodex
	pkg := s.cachedPackage
	cachedDeps := s.cachedDeps
	cachedWSL := s.cachedWSL
	cachedEngine := s.cachedEngine
	cachedEnvironment := s.cachedEnvironment
	cachedUpstream := s.cachedUpstream
	updateStatus := s.updateStatus
	events := append([]model.Event(nil), s.events...)
	testReport := s.testReport
	diagnostic := s.diagnostic
	initError := s.initError
	s.mu.RUnlock()

	codexConfig := filepath.Join(paths.CodexHome, "config.toml")
	engine := cachedEngine
	if deep || engine.Resolved == "" {
		var err error
		engine, err = codex.ResolveEngine(cfg, codexConfig)
		if err != nil {
			engine.Mode = cfg.Engine
			engine.ConfigPath = codexConfig
			engine.Error = err.Error()
		}
	}

	codexStatus := cachedCodex
	if deep {
		located, err := codex.Locate(ctx)
		if err != nil {
			codexStatus = model.CodexStatus{Error: err.Error()}
			pkg = codex.PackageInfo{}
			s.mu.Lock()
			s.cachedPackage = codex.PackageInfo{}
			s.mu.Unlock()
		} else {
			pkg = located
			s.mu.Lock()
			s.cachedPackage = pkg
			s.mu.Unlock()
		}
	}
	if deep && pkg.InstallLocation != "" {
		processes, processErr := codex.ListProcesses(ctx, pkg.InstallLocation)
		if processErr != nil {
			codexStatus = pkg.Status()
			codexStatus.Error = processErr.Error()
		} else {
			codexStatus = codex.StatusWithProcesses(pkg, processes)
		}
	}

	wslStatus := cachedWSL
	if deep || (engine.Resolved == model.EngineWSL && cachedWSL.Distro == "") {
		if engine.Resolved == model.EngineWSL {
			wslStatus = preflight.Inspect(ctx, paths.UserHome, paths.CodexHome, cfg.WSLDistro)
		} else {
			wslStatus = cachedWSL
		}
	}

	environment := cachedEnvironment
	if deep || cachedEnvironment.OSName == "" {
		environment = platform.InspectEnvironment(ctx, paths)
	}

	upstream := cachedUpstream
	if allowProbe && deep {
		probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
		upstream = s.probeUpstream(probeCtx, cfg)
		cancel()
	}

	laneStatus := model.LaneStatus{}
	if lane != nil {
		laneStatus = model.LaneStatus{
			Running:       true,
			InterfaceURL:  lane.InterfaceURL(),
			KernelURL:     lane.KernelURL(),
			InterfacePort: lane.InterfacePort(),
			KernelPort:    lane.KernelPort(),
			StartedAt:     lane.StartedAt().Format(time.RFC3339),
		}
	}
	if codexStatus.Running {
		codexStatus.ProxyMatched = laneStatus.Running &&
			codexStatus.ProxyURL != "" &&
			strings.EqualFold(codexStatus.ProxyURL, laneStatus.InterfaceURL)
	}

	stats := model.RequestStats{}
	var requests []model.RequestRecord
	var series []model.RequestSeriesPoint
	if store != nil {
		stats = store.Stats()
		requests = store.List(300)
		series = store.Series()
	}
	if initError != "" {
		laneStatus.Error = initError
	}
	if deep {
		s.updateDeepCache(environment, codexStatus, engine, wslStatus)
	}

	snapshot := model.Snapshot{
		Version:       Version,
		RepositoryURL: RepositoryURL,
		ReleasesURL:   ReleasesURL,
		Install:       installState(),
		Settings:      config.Settings(cfg),
		Lane:          laneStatus,
		Upstream:      upstream,
		Codex:         codexStatus,
		Dependencies:  cachedDeps,
		Environment:   environment,
		Engine:        engine,
		WSL:           wslStatus,
		Stats:         stats,
		Requests:      requests,
		Series:        series,
		TestReport:    testReport,
		Diagnostics:   diagnostic,
		Update:        updateStatus,
		Events:        events,
	}
	snapshot.Settings.LaunchAtLogin = platform.IsAutoStart()
	return snapshot
}

func gatewayTCPCheck(ctx context.Context, address string) (time.Duration, error) {
	return gateway.CheckTCPService(ctx, address)
}

func upstreamTunnelCheck(ctx context.Context, upstream model.Upstream, target string) (time.Duration, error) {
	return gateway.CheckUpstreamTunnel(ctx, upstream, target)
}

func (s *AppService) probeUpstream(ctx context.Context, cfg model.Config) model.UpstreamStatus {
	status := model.UpstreamStatus{
		URL:       config.UpstreamURL(cfg.Upstream),
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	address := net.JoinHostPort(cfg.Upstream.Host, strconv.Itoa(cfg.Upstream.Port))
	started := time.Now()
	if _, err := gatewayTCPCheck(ctx, address); err != nil {
		status.LatencyMs = time.Since(started).Milliseconds()
		status.Error = err.Error()
		s.cacheProbe(status)
		return status
	}
	tunnelDuration, err := upstreamTunnelCheck(ctx, cfg.Upstream, "auth.openai.com:443")
	status.LatencyMs = time.Since(started).Milliseconds()
	if err != nil {
		status.Error = err.Error()
	} else {
		status.Reachable = true
		status.LatencyMs = tunnelDuration.Milliseconds()
	}
	if status.Reachable {
		status.Domains = s.probeDomains(ctx, cfg)
		passed := 0
		for _, domain := range status.Domains {
			if domain.OK && !domain.Optional {
				passed++
			}
		}
		if passed == 0 {
			status.Reachable = false
			status.Error = "代理端口可连，但无法访问 OpenAI 域名"
		}
	}
	s.cacheProbe(status)
	return status
}

type domainTarget struct {
	host     string
	optional bool
}

var probeTargets = []domainTarget{
	{host: "openai.com"},
	{host: "chatgpt.com"},
	{host: "oaistatic.com"},
	{host: "files.oaiusercontent.com"},
	{host: "livekit.cloud", optional: true},
	{host: "sora.com", optional: true},
}

func (s *AppService) probeDomains(ctx context.Context, cfg model.Config) []model.DomainProbe {
	results := make([]model.DomainProbe, len(probeTargets))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for index, target := range probeTargets {
		wg.Add(1)
		go func(index int, target domainTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			probe := model.DomainProbe{Host: target.host, Optional: target.optional}
			probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			status, version, latency, err := gateway.CheckHostTLS(probeCtx, cfg.Upstream, target.host)
			probe.LatencyMs = latency.Milliseconds()
			probe.Status = status
			probe.TLS = version
			if err != nil {
				probe.Error = err.Error()
			} else {
				probe.OK = true
			}
			results[index] = probe
		}(index, target)
	}
	wg.Wait()
	return results
}

func (s *AppService) cacheProbe(status model.UpstreamStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status.URL != config.UpstreamURL(s.cfg.Upstream) {
		return
	}
	s.cachedUpstream = status
}

func (s *AppService) updateDeepCache(environment model.Environment, codexStatus model.CodexStatus, engine model.ResolvedEngine, wsl model.WSLStatus) {
	s.mu.Lock()
	s.cachedEnvironment = environment
	s.cachedCodex = codexStatus
	s.cachedEngine = engine
	s.cachedWSL = wsl
	s.mu.Unlock()
}
