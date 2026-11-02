package appservice

import (
	"context"
	"fmt"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/gateway"
	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func (s *AppService) ProbeAllUpstreams() []model.UpstreamProbeResult {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	results := make([]model.UpstreamProbeResult, 0, len(cfg.Upstreams))
	for _, profile := range cfg.Upstreams {
		result := model.UpstreamProbeResult{
			Name:     profile.Name,
			Protocol: profile.Protocol,
			Host:     profile.Host,
			Port:     profile.Port,
		}
		candidate := model.Upstream{Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port}
		probeCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		latency, err := gateway.CheckUpstreamTunnel(probeCtx, candidate, "auth.openai.com:443")
		cancel()
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Reachable = true
			result.LatencyMs = latency.Milliseconds()
		}
		results = append(results, result)
	}
	return results
}

func (s *AppService) SelectFastestUpstream() (model.Snapshot, error) {
	probes := s.ProbeAllUpstreams()
	best := -1
	for index, probe := range probes {
		if !probe.Reachable {
			continue
		}
		if best < 0 || probe.LatencyMs < probes[best].LatencyMs {
			best = index
		}
	}
	if best < 0 {
		return model.Snapshot{}, fmt.Errorf("没有可用的上游档案，请检查代理是否在线")
	}
	winner := probes[best]
	s.mu.RLock()
	cfg := s.cfg
	profile := model.UpstreamProfile{}
	for _, item := range cfg.Upstreams {
		if item.Name == winner.Name {
			profile = item
		}
	}
	s.mu.RUnlock()
	if profile.Name == "" {
		return model.Snapshot{}, fmt.Errorf("upstream profile %q does not exist", winner.Name)
	}
	if err := s.switchToProfile(profile, model.Upstream{Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port}); err != nil {
		return model.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return s.snapshot(ctx, false), nil
}

func (s *AppService) SwitchUpstream(name string) (model.Snapshot, error) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	var profile model.UpstreamProfile
	for _, item := range cfg.Upstreams {
		if item.Name == name {
			profile = item
		}
	}
	if profile.Name == "" {
		return model.Snapshot{}, fmt.Errorf("upstream profile %q does not exist", name)
	}
	if profile.Name == cfg.ActiveUpstream {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return s.snapshot(ctx, false), nil
	}
	if err := s.switchToProfile(profile, model.Upstream{Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port, NoProxy: cfg.Upstream.NoProxy}); err != nil {
		return model.Snapshot{}, err
	}
	s.addEvent("ok", "已切换上游代理", config.UpstreamURL(model.Upstream{Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port}))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return s.snapshot(ctx, false), nil
}
