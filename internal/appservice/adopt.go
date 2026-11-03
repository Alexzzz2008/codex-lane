package appservice

import (
	"context"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/codex"
	"github.com/Alexzzz2008/codex-lane/internal/model"
)

const (
	adoptInterval  = 10 * time.Second
	adoptRetryGap  = 2 * time.Minute
	adoptProbeTime = 12 * time.Second
	adoptGraceTime = 90 * time.Second
)

// startCodexWatcher adopts Codex windows that were opened from the Start menu
// or the desktop shortcut. Such a process starts without the Lane proxy flags,
// so it would send traffic straight to the internet; the watcher restarts it
// through Lane instead.
func (s *AppService) startCodexWatcher() {
	if s.ctx == nil || s.selfTest {
		return
	}
	go func() {
		ticker := time.NewTicker(adoptInterval)
		defer ticker.Stop()
		attempted := map[int]time.Time{}
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
			s.adoptNativeCodex(attempted)
		}
	}()
}

func (s *AppService) adoptNativeCodex(attempted map[int]time.Time) {
	cfg := s.currentConfig()
	if !cfg.Startup.AdoptCodex {
		return
	}
	lane := s.currentLane()
	if lane == nil {
		return
	}
	s.mu.RLock()
	launched := s.lastCodexLaunch
	s.mu.RUnlock()
	if !launched.IsZero() && time.Since(launched) < adoptGraceTime {
		// Codex may need a few seconds before it reports the proxy flags.
		return
	}
	for pid, when := range attempted {
		if time.Since(when) > 10*adoptRetryGap {
			delete(attempted, pid)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), adoptProbeTime)
	defer cancel()
	pkg, err := codex.Locate(ctx)
	if err != nil {
		return
	}
	processes, err := codex.ListProcesses(ctx, pkg.InstallLocation)
	if err != nil {
		return
	}
	status := codex.StatusWithProcesses(pkg, processes)
	if !status.Running || status.MainPID <= 0 {
		return
	}
	for _, process := range processes {
		if strings.EqualFold(codex.ProxyServer(process.CommandLine), lane.InterfaceURL()) {
			return
		}
	}
	if when, ok := attempted[status.MainPID]; ok && time.Since(when) < adoptRetryGap {
		return
	}
	attempted[status.MainPID] = time.Now()

	s.addEvent("info", "接管原生 Codex", "检测到 Codex 未经过 Lane，正在重启接入")
	if _, err := s.LaunchCodex(model.LaunchOptions{RestartExisting: true}); err != nil {
		s.addEvent("error", "接管 Codex 失败", err.Error())
		return
	}
	s.addEvent("ok", "已接管 Codex", "Codex 已通过 Lane 重新启动")
}
