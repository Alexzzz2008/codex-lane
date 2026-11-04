package appservice

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/codex"
	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
)

func newProxyTestService(t *testing.T) *AppService {
	t.Helper()
	service := New()
	ctx, cancel := context.WithCancel(context.Background())
	service.ctx = ctx
	service.cfg = config.Default()
	service.cfg.Startup.PromptCodexProxy = false
	service.paths.ConfigPath = filepath.Join(t.TempDir(), "launcher.json")
	service.paths.CodexHome = t.TempDir()
	service.cachedEngine = model.ResolvedEngine{Resolved: model.EngineWindows}
	service.cachedEnvironment = model.Environment{OSName: "Windows"}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service.cfg.Upstream.Port = listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	service.cfg.Upstreams = []model.UpstreamProfile{
		{Name: "primary", Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: service.cfg.Upstream.Port},
		{Name: "backup", Protocol: model.ProtocolSOCKS5, Host: "127.0.0.1", Port: service.cfg.Upstream.Port},
	}
	service.cfg.ActiveUpstream = "primary"
	t.Cleanup(func() {
		cancel()
		service.StopLane()
	})
	return service
}

func TestProfileProbesRunInParallelAndKeepProfileOrder(t *testing.T) {
	service := newProxyTestService(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{}, 4)
	release := make(chan struct{})
	defer close(release)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
					return
				}
				accepted <- struct{}{}
				select {
				case <-release:
				case <-service.ctx.Done():
					return
				}
				_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
			}()
		}
	}()
	service.cfg.Upstreams = nil
	for index := 0; index < 4; index++ {
		service.cfg.Upstreams = append(service.cfg.Upstreams, model.UpstreamProfile{
			Name: fmt.Sprintf("profile-%d", index), Protocol: model.ProtocolHTTP,
			Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port,
		})
	}
	done := make(chan []model.UpstreamProbeResult, 1)
	go func() { done <- service.ProbeAllUpstreams() }()
	for index := 0; index < 4; index++ {
		select {
		case <-accepted:
		case <-time.After(time.Second):
			t.Fatal("profile checks were serialized")
		}
	}
	for index := 0; index < 4; index++ {
		release <- struct{}{}
	}
	select {
	case results := <-done:
		for index, result := range results {
			if !result.Reachable || result.Name != fmt.Sprintf("profile-%d", index) {
				t.Fatalf("probe result order or reachability is incorrect: %+v", results)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("profile checks did not finish")
	}
}

func TestLocalProxyStartsEvenWhenUpstreamIsOffline(t *testing.T) {
	service := newProxyTestService(t)
	var pending sync.WaitGroup
	for index := 0; index < 8; index++ {
		pending.Add(1)
		go func() {
			defer pending.Done()
			if err := service.StartLane(); err != nil {
				t.Errorf("local proxy could not start: %v", err)
			}
		}()
	}
	pending.Wait()
	lane := service.currentLane()
	if lane == nil || lane.InterfacePort() == 0 || lane.KernelPort() == 0 {
		t.Fatal("local proxy did not open both endpoints")
	}
	saved, err := config.Load(service.paths.ConfigPath)
	if err != nil || saved.Lane.InterfacePort != lane.InterfacePort() || saved.Lane.KernelPort != lane.KernelPort() {
		t.Fatalf("local ports were not remembered: %+v, error=%v", saved.Lane, err)
	}
}

func TestSwitchWhileStoppedDoesNotStartLocalProxy(t *testing.T) {
	service := newProxyTestService(t)
	snapshot, err := service.SwitchUpstream("backup")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Lane.Running || service.currentLane() != nil || snapshot.Settings.ActiveUpstream != "backup" {
		t.Fatal("switching a stopped proxy changed its run state")
	}
}

func TestSettingsAndProfileSwitchKeepExistingListener(t *testing.T) {
	service := newProxyTestService(t)
	if err := service.StartLane(); err != nil {
		t.Fatal(err)
	}
	lane := service.currentLane()
	if _, err := service.SwitchUpstream("backup"); err != nil {
		t.Fatal(err)
	}
	input := model.SettingsInput(config.Settings(service.currentConfig()))
	input.LaunchAtLogin = platform.IsAutoStart()
	input.Host = "127.0.0.2"
	if _, err := service.SaveSettings(input); err != nil {
		t.Fatal(err)
	}
	if service.currentLane() != lane {
		t.Fatal("changing upstream replaced the local listener")
	}
}

func TestFailedProfileSaveKeepsCurrentProxy(t *testing.T) {
	service := newProxyTestService(t)
	service.paths.ConfigPath = t.TempDir()
	service.cachedUpstream = model.UpstreamStatus{URL: config.UpstreamURL(service.cfg.Upstream), Reachable: true}
	if _, err := service.SwitchUpstream("backup"); err == nil {
		t.Fatal("saving over a directory should fail")
	}
	if service.cfg.ActiveUpstream != "primary" || !service.cachedUpstream.Reachable {
		t.Fatal("failed save changed the active proxy or its status")
	}
}

func TestSavingManualEngineWritesAnAbsentSetting(t *testing.T) {
	service := newProxyTestService(t)
	service.cfg.Engine = model.EngineWSL
	service.cachedEngine = model.ResolvedEngine{Resolved: model.EngineWSL}
	service.cachedWSL = model.WSLStatus{Distro: "fixture"}
	input := model.SettingsInput(config.Settings(service.cfg))
	input.LaunchAtLogin = platform.IsAutoStart()
	if _, err := service.SaveSettings(input); err != nil {
		t.Fatal(err)
	}
	state, err := codex.ReadEngine(filepath.Join(service.paths.CodexHome, "config.toml"))
	if err != nil || !state.Present || !state.WSL {
		t.Fatalf("manual WSL selection was not written: %+v, error=%v", state, err)
	}
}

func TestInvalidConfigurationIsNotOverwrittenByPortMemory(t *testing.T) {
	service := newProxyTestService(t)
	broken := []byte("{broken user configuration")
	if err := os.WriteFile(service.paths.ConfigPath, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	service.initError = "invalid configuration"
	if err := service.StartLane(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(service.paths.ConfigPath)
	if err != nil || string(got) != string(broken) {
		t.Fatalf("configuration was overwritten: %q, %v", got, err)
	}
}

func TestFailedSettingsSaveDoesNotChangeCodexEngine(t *testing.T) {
	service := newProxyTestService(t)
	service.paths.ConfigPath = t.TempDir()
	input := model.SettingsInput(config.Settings(service.cfg))
	input.Engine = model.EngineWSL
	input.LaunchAtLogin = platform.IsAutoStart()
	if _, err := service.SaveSettings(input); err == nil {
		t.Fatal("expected a save failure")
	}
	state, err := codex.ReadEngine(filepath.Join(service.paths.CodexHome, "config.toml"))
	if err != nil || state.Present {
		t.Fatalf("failed save changed Codex: %+v, %v", state, err)
	}
}
