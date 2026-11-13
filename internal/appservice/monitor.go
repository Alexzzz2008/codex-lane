package appservice

import (
	"context"
	"fmt"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/gateway"
	"github.com/Alexzzz2008/codex-lane/internal/model"
)

const (
	monitorInterval     = 30 * time.Second
	retentionInterval   = 6 * time.Hour
	upstreamFailLimit   = 2
	codexDriftThreshold = 3
)

func (s *AppService) startMonitor() {
	if s.ctx == nil {
		return
	}
	go func() {
		s.cleanRetention()
		healthTick := time.NewTicker(monitorInterval)
		defer healthTick.Stop()
		retentionTick := time.NewTicker(retentionInterval)
		defer retentionTick.Stop()
		upstreamFails := 0
		upstreamAlerted := false
		monitoredUpstream := ""
		driftCount := 0
		driftAlerted := false
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-retentionTick.C:
				s.cleanRetention()
			case <-healthTick.C:
				if s.currentLane() == nil {
					upstreamFails = 0
					continue
				}
				cfg := s.currentConfig()
				upstreamURL := config.UpstreamURL(cfg.Upstream)
				if upstreamURL != monitoredUpstream {
					monitoredUpstream = upstreamURL
					upstreamFails = 0
					upstreamAlerted = false
				}
				err := s.checkUpstreamHealth(s.ctx, cfg)
				if s.currentLane() == nil || config.UpstreamURL(s.currentConfig().Upstream) != upstreamURL {
					continue
				}
				if err == nil {
					upstreamFails = 0
					upstreamAlerted = false
				} else {
					upstreamFails++
					if upstreamFails < upstreamFailLimit {
						continue
					}
					if cfg.AutoFailover && s.failoverUpstream() {
						upstreamFails = 0
						upstreamAlerted = false
						continue
					}
					if !upstreamAlerted {
						upstreamAlerted = true
						s.addEvent("error", "上游代理连接失败", err.Error())
					}
				}
				if driftAlerted = s.checkCodexDrift(&driftCount, driftAlerted); driftCount == 0 {
					driftAlerted = false
				}
			}
		}
	}()
}

func (s *AppService) checkUpstreamHealth(ctx context.Context, cfg model.Config) error {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	latency, err := gateway.CheckUpstreamTunnel(ctx, cfg.Upstream, "auth.openai.com:443")
	status := model.UpstreamStatus{
		URL:       config.UpstreamURL(cfg.Upstream),
		CheckedAt: time.Now().Format(time.RFC3339),
		Reachable: err == nil,
		LatencyMs: latency.Milliseconds(),
	}
	if err != nil {
		status.Error = err.Error()
	}
	s.cacheProbe(status)
	return err
}

func (s *AppService) cleanRetention() {
	s.mu.RLock()
	store := s.store
	days := s.cfg.Logging.RetentionDays
	s.mu.RUnlock()
	if store == nil {
		return
	}
	if removed := store.RemoveExpired(days); removed > 0 {
		s.addEvent("info", "历史日志已清理", fmt.Sprintf("删除 %d 个过期日志文件（保留 %d 天）", removed, days))
	}
}

func (s *AppService) checkCodexDrift(driftCount *int, alerted bool) bool {
	s.mu.RLock()
	codex := s.cachedCodex
	s.mu.RUnlock()
	if !codex.Running {
		*driftCount = 0
		return false
	}
	if !codex.ProxyMatched {
		*driftCount++
		if *driftCount >= codexDriftThreshold && !alerted {
			s.addEvent("warn", "Codex 未接入当前 Lane", "Codex 进程的代理参数与本地入口不一致，可尝试重新打开 Codex 接入")
			return true
		}
		return alerted
	}
	*driftCount = 0
	return false
}

func (s *AppService) failoverUpstream() bool {
	s.mu.RLock()
	cfg := s.cfg
	lane := s.lane
	s.mu.RUnlock()
	if !cfg.AutoFailover || lane == nil {
		return false
	}
	for _, profile := range cfg.Upstreams {
		if profile.Name == cfg.ActiveUpstream || profile.Protocol == "" {
			continue
		}
		candidate := model.Upstream{Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port, NoProxy: cfg.Upstream.NoProxy}
		probeCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		_, err := gateway.CheckUpstreamTunnel(probeCtx, candidate, "auth.openai.com:443")
		cancel()
		if err != nil {
			continue
		}
		s.operationMu.Lock()
		s.mu.RLock()
		unchanged := s.cfg.AutoFailover && s.lane == lane &&
			s.cfg.ActiveUpstream == cfg.ActiveUpstream && s.cfg.Upstream == cfg.Upstream
		s.mu.RUnlock()
		if !unchanged {
			s.operationMu.Unlock()
			return false
		}
		err = s.switchToProfileLocked(profile)
		s.operationMu.Unlock()
		if err != nil {
			continue
		}
		s.addEvent("ok", "已自动切换上游代理", config.UpstreamURL(candidate))
		return true
	}
	return false
}

func (s *AppService) switchToProfile(profile model.UpstreamProfile) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.switchToProfileLocked(profile)
}

// switchToProfileLocked requires operationMu so an old health probe cannot undo a user change.
func (s *AppService) switchToProfileLocked(profile model.UpstreamProfile) error {
	s.mu.Lock()
	if s.paths.ConfigPath == "" {
		s.mu.Unlock()
		return fmt.Errorf("app service is not ready")
	}
	cfg := s.cfg
	path := s.paths.ConfigPath
	s.mu.Unlock()
	found := false
	for _, current := range cfg.Upstreams {
		if current == profile {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("代理档案已变更，请重新选择")
	}
	cfg.ActiveUpstream = profile.Name
	cfg.Upstream.Protocol = profile.Protocol
	cfg.Upstream.Host = profile.Host
	cfg.Upstream.Port = profile.Port
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.cachedUpstream = model.UpstreamStatus{URL: config.UpstreamURL(cfg.Upstream)}
	s.invalidateChecksLocked()
	s.mu.Unlock()
	s.applyLaneRouting(cfg)
	return nil
}
