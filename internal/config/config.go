package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

const (
	DefaultNoProxy = "localhost,127.0.0.1,::1"
	FileName       = "launcher.json"
)

func Default() model.Config {
	return model.Config{
		SchemaVersion: 1,
		Upstream: model.Upstream{
			Protocol: model.ProtocolHTTP,
			Host:     "127.0.0.1",
			Port:     7897,
			NoProxy:  DefaultNoProxy,
		},
		Lane: model.Lane{
			ListenHost: "127.0.0.1",
			ListenPort: 0,
		},
		Engine:              model.EngineAuto,
		AskBeforeKill:       true,
		RequireHealthyProxy: true,
		Startup: model.Startup{
		OpenCodex:       false,
		StopCodexOnExit: false,
		MinimizeToTray:  true,
		},
		Logging: model.Logging{
			RequestMetadata: true,
			RetentionDays:   14,
			MaxMemoryRows:   10000,
		},
	}
}

func Load(path string) (model.Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := Save(path, cfg); err != nil {
			return cfg, fmt.Errorf("create default config: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	Normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Save(path string, cfg model.Config) error {
	Normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	return writeAtomic(path, data, 0o600)
}

// MigrateLegacyConfig moves the first valid launcher.json found in candidates
// to target. Once target exists, remaining legacy files are removed.
func MigrateLegacyConfig(target string, candidates ...string) error {
	target = filepath.Clean(target)
	seen := map[string]bool{target: true}
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		unique = append(unique, candidate)
	}

	if _, err := os.Stat(target); err == nil {
		return removeLegacyConfigs(unique...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect config: %w", err)
	}

	for _, candidate := range unique {
		data, err := os.ReadFile(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read legacy config %s: %w", candidate, err)
		}
		var cfg model.Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse legacy config %s: %w", candidate, err)
		}
		Normalize(&cfg)
		if err := Validate(cfg); err != nil {
			return fmt.Errorf("validate legacy config %s: %w", candidate, err)
		}
		if err := Save(target, cfg); err != nil {
			return fmt.Errorf("migrate config: %w", err)
		}
		break
	}
	return removeLegacyConfigs(unique...)
}

func removeLegacyConfigs(paths ...string) error {
	var result error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, fmt.Errorf("remove legacy config %s: %w", path, err))
		}
	}
	return result
}

func Normalize(cfg *model.Config) {
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = 1
	}
	cfg.Upstream.Protocol = strings.ToLower(strings.TrimSpace(cfg.Upstream.Protocol))
	cfg.Upstream.Host = strings.TrimSpace(cfg.Upstream.Host)
	cfg.Upstream.NoProxy = strings.TrimSpace(cfg.Upstream.NoProxy)
	cfg.Engine = strings.ToLower(strings.TrimSpace(cfg.Engine))
	cfg.WSLDistro = strings.TrimSpace(cfg.WSLDistro)
	cfg.Lane.ListenHost = strings.TrimSpace(cfg.Lane.ListenHost)
	cfg.Lane.HostFilterMode = strings.ToLower(strings.TrimSpace(cfg.Lane.HostFilterMode))
	cfg.ActiveUpstream = strings.TrimSpace(cfg.ActiveUpstream)

	if cfg.Upstream.Protocol == "" {
		cfg.Upstream.Protocol = model.ProtocolHTTP
	}
	if cfg.Upstream.Host == "" {
		cfg.Upstream.Host = "127.0.0.1"
	}
	if cfg.Upstream.Port == 0 {
		cfg.Upstream.Port = 7897
	}
	if cfg.Upstream.NoProxy == "" {
		cfg.Upstream.NoProxy = DefaultNoProxy
	}
	if cfg.Engine == "" {
		cfg.Engine = model.EngineAuto
	}
	if cfg.Lane.ListenHost == "" || cfg.Lane.ListenHost == "localhost" {
		cfg.Lane.ListenHost = "127.0.0.1"
	}
	if cfg.Lane.HostFilterMode == "" {
		cfg.Lane.HostFilterMode = model.HostFilterOff
	}
	if cfg.Lane.AllowedHosts == nil {
		cfg.Lane.AllowedHosts = []string{}
	}
	if cfg.Lane.Routes == nil {
		cfg.Lane.Routes = []model.RouteRule{}
	}
	if cfg.Upstreams == nil {
		cfg.Upstreams = []model.UpstreamProfile{}
	}
	if cfg.Logging.RetentionDays <= 0 {
		cfg.Logging.RetentionDays = 14
	}
	if cfg.Logging.MaxMemoryRows <= 0 {
		cfg.Logging.MaxMemoryRows = 10000
	}
}

func Validate(cfg model.Config) error {
	switch cfg.Upstream.Protocol {
	case model.ProtocolHTTP, model.ProtocolHTTPS, model.ProtocolSOCKS5:
	default:
		return fmt.Errorf("unsupported proxy protocol %q", cfg.Upstream.Protocol)
	}
	if err := validateHost(cfg.Upstream.Host); err != nil {
		return fmt.Errorf("upstream host: %w", err)
	}
	if cfg.Upstream.Port < 1 || cfg.Upstream.Port > 65535 {
		return fmt.Errorf("upstream port must be between 1 and 65535")
	}
	if cfg.Lane.ListenHost != "127.0.0.1" && cfg.Lane.ListenHost != "::1" {
		return fmt.Errorf("lane listen host must be loopback")
	}
	if cfg.Lane.ListenPort < 0 || cfg.Lane.ListenPort > 65535 {
		return fmt.Errorf("lane listen port must be between 0 and 65535")
	}
	switch cfg.Lane.HostFilterMode {
	case model.HostFilterOff, model.HostFilterAllowlist:
	default:
		return fmt.Errorf("unsupported host filter mode %q", cfg.Lane.HostFilterMode)
	}
	for _, pattern := range cfg.Lane.AllowedHosts {
		if err := validateHostPattern(pattern); err != nil {
			return fmt.Errorf("allowed host %q: %w", pattern, err)
		}
	}
	for _, rule := range cfg.Lane.Routes {
		if err := validateHostPattern(rule.Pattern); err != nil {
			return fmt.Errorf("route pattern %q: %w", rule.Pattern, err)
		}
		switch rule.Action {
		case model.RouteProxy, model.RouteDirect:
		default:
			return fmt.Errorf("route action %q must be proxy or direct", rule.Action)
		}
	}
	seenProfiles := make(map[string]bool, len(cfg.Upstreams))
	for _, profile := range cfg.Upstreams {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			return fmt.Errorf("upstream profile name is required")
		}
		if seenProfiles[name] {
			return fmt.Errorf("duplicate upstream profile name %q", name)
		}
		seenProfiles[name] = true
		switch profile.Protocol {
		case model.ProtocolHTTP, model.ProtocolHTTPS, model.ProtocolSOCKS5:
		default:
			return fmt.Errorf("upstream profile %q: unsupported protocol %q", name, profile.Protocol)
		}
		if err := validateHost(profile.Host); err != nil {
			return fmt.Errorf("upstream profile %q host: %w", name, err)
		}
		if profile.Port < 1 || profile.Port > 65535 {
			return fmt.Errorf("upstream profile %q port must be between 1 and 65535", name)
		}
	}
	if cfg.ActiveUpstream != "" && !seenProfiles[cfg.ActiveUpstream] {
		return fmt.Errorf("active upstream profile %q does not exist", cfg.ActiveUpstream)
	}
	switch cfg.Engine {
	case model.EngineAuto, model.EngineWindows, model.EngineWSL:
	default:
		return fmt.Errorf("unsupported engine mode %q", cfg.Engine)
	}
	if cfg.Logging.RetentionDays < 1 || cfg.Logging.RetentionDays > 365 {
		return fmt.Errorf("retention days must be between 1 and 365")
	}
	if cfg.Logging.MaxMemoryRows < 100 || cfg.Logging.MaxMemoryRows > 100000 {
		return fmt.Errorf("max memory rows must be between 100 and 100000")
	}
	for _, ch := range cfg.WSLDistro {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' && ch != '.' {
			return fmt.Errorf("invalid WSL distro name")
		}
	}
	return nil
}

func validateHostPattern(pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "*" {
		return nil
	}
	if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
		return validateHost(suffix)
	}
	return validateHost(pattern)
}

func validateHost(host string) error {
	if host == "" {
		return errors.New("host is required")
	}
	if strings.ContainsAny(host, "/\\?#@ \t\r\n") {
		return errors.New("host must not contain a URL, path, credentials or whitespace")
	}
	if strings.Contains(host, ":") {
		if ip := net.ParseIP(host); ip == nil {
			return errors.New("invalid IPv6 address")
		}
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return nil
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return errors.New("invalid hostname")
		}
		for i, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && !(r == '_' && i > 0) {
				return errors.New("invalid hostname")
			}
		}
	}
	return nil
}

func ApplySettings(cfg *model.Config, in model.SettingsInput) error {
	profiles := make([]model.UpstreamProfile, 0, len(in.Upstreams))
	for _, profile := range in.Upstreams {
		profiles = append(profiles, model.UpstreamProfile{
			Name:     strings.TrimSpace(profile.Name),
			Protocol: strings.ToLower(strings.TrimSpace(profile.Protocol)),
			Host:     strings.TrimSpace(profile.Host),
			Port:     profile.Port,
		})
	}
	active := strings.TrimSpace(in.ActiveUpstream)
	cfg.Upstreams = profiles
	cfg.ActiveUpstream = active
	if profile := findProfile(profiles, active); profile != nil {
		cfg.Upstream.Protocol = profile.Protocol
		cfg.Upstream.Host = profile.Host
		cfg.Upstream.Port = profile.Port
	} else {
		cfg.Upstream.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
		cfg.Upstream.Host = strings.TrimSpace(in.Host)
		cfg.Upstream.Port = in.Port
	}
	cfg.Upstream.NoProxy = strings.TrimSpace(in.NoProxy)
	cfg.Engine = strings.ToLower(strings.TrimSpace(in.Engine))
	cfg.WSLDistro = strings.TrimSpace(in.WSLDistro)
	cfg.Startup.OpenCodex = in.OpenCodex
	cfg.Startup.StopCodexOnExit = in.StopCodexOnExit
	cfg.Startup.MinimizeToTray = in.MinimizeToTray
	cfg.AutoFailover = in.AutoFailover
	cfg.Lane.HostFilterMode = strings.ToLower(strings.TrimSpace(in.HostFilterMode))
	cfg.Lane.AllowedHosts = trimPatterns(in.AllowedHosts)
	routes := make([]model.RouteRule, 0, len(in.Routes))
	for _, rule := range in.Routes {
		routes = append(routes, model.RouteRule{
			Pattern: strings.TrimSpace(rule.Pattern),
			Action:  strings.ToLower(strings.TrimSpace(rule.Action)),
		})
	}
	cfg.Lane.Routes = routes
	if in.RetentionDays > 0 {
		cfg.Logging.RetentionDays = in.RetentionDays
	}
	if in.MaxMemoryRows > 0 {
		cfg.Logging.MaxMemoryRows = in.MaxMemoryRows
	}
	Normalize(cfg)
	return Validate(*cfg)
}

func findProfile(profiles []model.UpstreamProfile, name string) *model.UpstreamProfile {
	if name == "" {
		return nil
	}
	for index := range profiles {
		if profiles[index].Name == name {
			return &profiles[index]
		}
	}
	return nil
}

func trimPatterns(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" {
			out = append(out, pattern)
		}
	}
	return out
}

func Settings(cfg model.Config) model.Settings {
	return model.Settings{
		Protocol:        cfg.Upstream.Protocol,
		Host:            cfg.Upstream.Host,
		Port:            cfg.Upstream.Port,
		NoProxy:         cfg.Upstream.NoProxy,
		Engine:          cfg.Engine,
		WSLDistro:       cfg.WSLDistro,
		OpenCodex:       cfg.Startup.OpenCodex,
		StopCodexOnExit: cfg.Startup.StopCodexOnExit,
		RetentionDays:   cfg.Logging.RetentionDays,
		MaxMemoryRows:   cfg.Logging.MaxMemoryRows,
		Upstreams:       cfg.Upstreams,
		ActiveUpstream:  cfg.ActiveUpstream,
		AutoFailover:    cfg.AutoFailover,
		HostFilterMode:  cfg.Lane.HostFilterMode,
		AllowedHosts:    cfg.Lane.AllowedHosts,
		Routes:          cfg.Lane.Routes,
		MinimizeToTray:  cfg.Startup.MinimizeToTray,
	}
}

func UpstreamURL(u model.Upstream) string {
	return u.Protocol + "://" + net.JoinHostPort(u.Host, strconv.Itoa(u.Port))
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("set temporary config permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
