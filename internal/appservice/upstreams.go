package appservice

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/gateway"
	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func (s *AppService) ProbeAllUpstreams() []model.UpstreamProbeResult {
	s.mu.RLock()
	cfg := s.cfg
	ctx := s.ctx
	s.mu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	results := make([]model.UpstreamProbeResult, len(cfg.Upstreams))
	limit := make(chan struct{}, 4)
	var pending sync.WaitGroup
	for index, profile := range cfg.Upstreams {
		pending.Add(1)
		go func() {
			defer pending.Done()
			result := model.UpstreamProbeResult{
				Name:     profile.Name,
				Protocol: profile.Protocol,
				Host:     profile.Host,
				Port:     profile.Port,
			}
			defer func() { results[index] = result }()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				result.Error = ctx.Err().Error()
				return
			}
			candidate := model.Upstream{Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port}
			latency, err := gateway.CheckUpstreamTunnel(ctx, candidate, "auth.openai.com:443")
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Reachable = true
				result.LatencyMs = latency.Milliseconds()
			}
		}()
	}
	pending.Wait()
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
		if item.Name == winner.Name && item.Protocol == winner.Protocol && item.Host == winner.Host && item.Port == winner.Port {
			profile = item
		}
	}
	s.mu.RUnlock()
	if profile.Name == "" {
		return model.Snapshot{}, fmt.Errorf("upstream profile %q does not exist", winner.Name)
	}
	if err := s.switchToProfile(profile); err != nil {
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
	if err := s.switchToProfile(profile); err != nil {
		return model.Snapshot{}, err
	}
	s.addEvent("ok", "已切换上游代理", config.UpstreamURL(model.Upstream{Protocol: profile.Protocol, Host: profile.Host, Port: profile.Port}))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return s.snapshot(ctx, false), nil
}
