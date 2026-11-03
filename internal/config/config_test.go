package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestLoadAdoptsCodexDefaultForLegacyConfig(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "launcher.json")
	legacy := `{"schemaVersion": 1, "startup": {"openCodex": false, "minimizeToTray": true}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Startup.AdoptCodex {
		t.Fatal("legacy config should enable Codex adoption by default")
	}
}

func TestLoadKeepsExplicitAdoptCodexOff(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "launcher.json")
	disabled := `{"schemaVersion": 1, "startup": {"openCodex": false, "adoptCodex": false}}`
	if err := os.WriteFile(path, []byte(disabled), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Startup.AdoptCodex {
		t.Fatal("explicitly disabled adoption must stay off")
	}
}

func TestApplySettingsUpdatesActiveProfileFromTopFields(t *testing.T) {
	cfg := Default()
	cfg.Upstreams = []model.UpstreamProfile{{
		Name:     "档案 1",
		Protocol: model.ProtocolHTTP,
		Host:     "127.0.0.1",
		Port:     7897,
	}}
	cfg.ActiveUpstream = "档案 1"
	cfg.Upstream = model.Upstream{Protocol: model.ProtocolHTTP, Host: "127.0.0.1", Port: 7897}

	input := model.SettingsInput{
		Protocol:        model.ProtocolSOCKS5,
		Host:            "10.0.0.2",
		Port:            1080,
		NoProxy:         cfg.Upstream.NoProxy,
		Engine:          model.EngineAuto,
		Upstreams:       cfg.Upstreams,
		ActiveUpstream:  "档案 1",
		HostFilterMode:  model.HostFilterOff,
		RetentionDays:   cfg.Logging.RetentionDays,
		MaxMemoryRows:   cfg.Logging.MaxMemoryRows,
		MinimizeToTray:  cfg.Startup.MinimizeToTray,
		AllowedHosts:    []string{},
		Routes:          []model.RouteRule{},
		AutoFailover:    cfg.AutoFailover,
		StopCodexOnExit: cfg.Startup.StopCodexOnExit,
		OpenCodex:       cfg.Startup.OpenCodex,
	}
	if err := ApplySettings(&cfg, input); err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}
	if cfg.Upstream.Protocol != model.ProtocolSOCKS5 || cfg.Upstream.Host != "10.0.0.2" || cfg.Upstream.Port != 1080 {
		t.Fatalf("top-level upstream = %+v", cfg.Upstream)
	}
	if len(cfg.Upstreams) != 1 || cfg.Upstreams[0].Protocol != model.ProtocolSOCKS5 || cfg.Upstreams[0].Host != "10.0.0.2" || cfg.Upstreams[0].Port != 1080 {
		t.Fatalf("active profile not updated: %+v", cfg.Upstreams)
	}
}
