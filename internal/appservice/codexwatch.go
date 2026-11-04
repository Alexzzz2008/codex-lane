package appservice

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/codex"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
)

//wails:ignore
func (s *AppService) SetShowWindow(show func()) {
	s.mu.Lock()
	s.showWindow = show
	s.mu.Unlock()
}

func (s *AppService) configureCodexWatch() {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	s.mu.RLock()
	enabled := s.cfg.Startup.PromptCodexProxy && !s.selfTest && s.ctx != nil
	parent := s.ctx
	initial := s.cachedCodex
	s.mu.RUnlock()
	if enabled && s.watchCancel != nil {
		return
	}
	if s.watchCancel != nil {
		s.watchCancel()
		s.watchCancel = nil
	}
	s.mu.Lock()
	s.codexWatchError = ""
	s.mu.Unlock()
	if !enabled {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.watchCancel = cancel
	events := make(chan int, 16)
	go func() {
		if err := platform.WatchWindowProcesses(ctx, events); err != nil && ctx.Err() == nil {
			s.mu.Lock()
			s.codexWatchError = err.Error()
			s.mu.Unlock()
			s.addEvent("warn", "原生 Codex 接入提示不可用", err.Error())
			cancel()
		}
	}()
	go func() {
		seen := make(map[int]bool)
		for _, process := range initial.Processes {
			seen[process.PID] = true
		}
		if initial.MainPID > 0 {
			seen[initial.MainPID] = true
		}
		s.offerCodexProxy(initial)
		for {
			select {
			case <-ctx.Done():
				return
			case pid := <-events:
				if seen[pid] || s.currentLane() == nil {
					continue
				}
				executable, err := platform.ProcessExecutable(pid)
				if err != nil || !isCodexExecutable(executable) {
					continue
				}
				if len(seen) >= 128 {
					clear(seen)
				}
				seen[pid] = true
				s.inspectCodexWindow(ctx, executable)
			}
		}
	}()
}

func isCodexExecutable(executable string) bool {
	name := filepath.Base(executable)
	return strings.EqualFold(name, "ChatGPT.exe") || strings.EqualFold(name, "Codex.exe")
}

func (s *AppService) inspectCodexWindow(ctx context.Context, executable string) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	s.mu.RLock()
	pkg := s.cachedPackage
	s.mu.RUnlock()
	if !strings.EqualFold(pkg.ExecutablePath, executable) {
		located, err := codex.Locate(ctx)
		if err != nil || !strings.EqualFold(located.ExecutablePath, executable) {
			return
		}
		pkg = located
	}
	if err := codex.ValidateCodexPath(executable, pkg.InstallLocation); err != nil {
		return
	}
	processes, err := codex.ListProcesses(ctx, pkg.InstallLocation)
	if err != nil || ctx.Err() != nil {
		return
	}
	status := codex.StatusWithProcesses(pkg, processes)
	s.mu.Lock()
	s.cachedPackage = pkg
	s.cachedCodex = status
	s.mu.Unlock()
	s.offerCodexProxy(status)
}

func (s *AppService) offerCodexProxy(status model.CodexStatus) {
	s.mu.Lock()
	lane := s.lane
	enabled := s.cfg.Startup.PromptCodexProxy
	emit := s.emitFn
	show := s.showWindow
	if !enabled || lane == nil || !status.Running || status.MainPID <= 0 {
		s.mu.Unlock()
		return
	}
	status.ProxyMatched = strings.EqualFold(status.ProxyURL, lane.InterfaceURL())
	if !status.ProxyMatched && s.lastCodexOffer == status.MainPID {
		s.mu.Unlock()
		return
	}
	if !status.ProxyMatched {
		s.lastCodexOffer = status.MainPID
	}
	s.mu.Unlock()
	if emit != nil {
		emit("lane:codex", status)
	}
	if !status.ProxyMatched && show != nil {
		show()
	}
}

func (s *AppService) stopCodexWatch() {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watchCancel != nil {
		s.watchCancel()
		s.watchCancel = nil
	}
}
