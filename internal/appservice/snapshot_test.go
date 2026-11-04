package appservice

import (
	"context"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/codex"
	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestShallowSnapshotDoesNotScanCodex(t *testing.T) {
	service := New()
	service.cfg = config.Default()
	service.cachedPackage = codex.PackageInfo{InstallLocation: t.TempDir()}
	service.cachedCodex = model.CodexStatus{Running: true, MainPID: 12345}
	service.cachedEngine = model.ResolvedEngine{Resolved: model.EngineWindows}
	service.cachedEnvironment = model.Environment{OSName: "Windows"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot := service.snapshotWith(ctx, false, false)
	if !snapshot.Codex.Running || snapshot.Codex.MainPID != 12345 {
		t.Fatalf("shallow snapshot replaced the cached process status: %+v", snapshot.Codex)
	}
}

func TestProbeOfUnsavedProxyDoesNotReplaceActiveStatus(t *testing.T) {
	service := New()
	service.cfg = config.Default()
	service.cachedUpstream = model.UpstreamStatus{URL: config.UpstreamURL(service.cfg.Upstream), Reachable: true}
	service.cacheProbe(model.UpstreamStatus{URL: "http://127.0.0.1:1234", Error: "candidate unavailable"})
	if !service.cachedUpstream.Reachable || service.cachedUpstream.URL != config.UpstreamURL(service.cfg.Upstream) {
		t.Fatalf("candidate overwrote active status: %+v", service.cachedUpstream)
	}
}
