package appservice

import (
	"testing"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestProxyOfferDoesNotRestartAndOnlyShowsOncePerPID(t *testing.T) {
	service := newProxyTestService(t)
	if err := service.StartLane(); err != nil {
		t.Fatal(err)
	}
	service.cfg.Startup.PromptCodexProxy = true
	shown, offered := 0, 0
	service.SetShowWindow(func() { shown++ })
	service.SetEmitter(func(name string, payload ...any) {
		if name == "lane:codex" {
			offered++
		}
	})
	status := model.CodexStatus{Running: true, MainPID: 12345}
	service.offerCodexProxy(status)
	service.offerCodexProxy(status)
	if shown != 1 || offered != 1 {
		t.Fatalf("repeat offer: shown=%d, offered=%d", shown, offered)
	}
	status.MainPID++
	service.offerCodexProxy(status)
	if shown != 2 {
		t.Fatal("a newly started Codex was not offered")
	}
	status.ProxyURL = service.currentLane().InterfaceURL()
	service.offerCodexProxy(status)
	if shown != 2 {
		t.Fatal("already connected Codex caused a restart prompt")
	}
	service.cfg.Startup.PromptCodexProxy = false
	status.MainPID++
	service.offerCodexProxy(status)
	if shown != 2 {
		t.Fatal("disabled discovery still showed a window")
	}
}
