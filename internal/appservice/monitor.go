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
				probeCtx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				_, err := gateway.CheckUpstreamTunnel(probeCtx, cfg.Upstream, "auth.openai.com:443")
				cancel()
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
	s.mu.RUnlock()
	for _, profile := range cfg.Upstreams {
		if profile.Name == cfg.ActiveUpstream || profile.Protocol == "" {
			continue
		}
		candidate := model.Upstream{Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port, NoProxy: cfg.Upstream.NoProxy}
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := gateway.CheckUpstreamTunnel(probeCtx, candidate, "auth.openai.com:443")
		cancel()
		if err != nil {
			continue
		}
		if err := s.switchToProfile(profile, candidate); err != nil {
			continue
		}
		s.addEvent("ok", "已自动切换上游代理", config.UpstreamURL(candidate))
		return true
	}
	return false
}

func (s *AppService) switchToProfile(profile model.UpstreamProfile, candidate model.Upstream) error {
	s.mu.Lock()
	if s.paths.ConfigPath == "" {
		s.mu.Unlock()
		return fmt.Errorf("app service is not ready")
	}
	cfg := s.cfg
	previous := s.cfg
	path := s.paths.ConfigPath
	cfg.ActiveUpstream = profile.Name
	cfg.Upstream.Protocol = candidate.Protocol
	cfg.Upstream.Host = candidate.Host
	cfg.Upstream.Port = candidate.Port
	s.cfg = cfg
	s.lastProbe = time.Time{}
	s.cachedUpstream = model.UpstreamStatus{URL: config.UpstreamURL(candidate)}
	s.testReport = nil
	s.mu.Unlock()
	if err := config.Save(path, cfg); err != nil {
		s.mu.Lock()
		s.cfg = previous
		s.mu.Unlock()
		return err
	}
	return s.restartLane()
}
