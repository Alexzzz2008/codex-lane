package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, FileName)
	target := filepath.Join(dir, "system", FileName)
	cfg := Default()
	cfg.Upstream.Port = 8111
	if err := Save(legacy, cfg); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyConfig(target, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists: %v", err)
	}
	migrated, err := Load(target)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Upstream.Port != 8111 {
		t.Fatalf("migrated port = %d, want 8111", migrated.Upstream.Port)
	}
}

func TestMigrateLegacyConfigKeepsExistingTarget(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, FileName)
	target := filepath.Join(dir, "system", FileName)
	legacyConfig := Default()
	legacyConfig.Upstream.Port = 8111
	if err := Save(legacy, legacyConfig); err != nil {
		t.Fatal(err)
	}
	targetConfig := Default()
	targetConfig.Upstream.Port = 9222
	if err := Save(target, targetConfig); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyConfig(target, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists: %v", err)
	}
	loaded, err := Load(target)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Upstream.Port != 9222 {
		t.Fatalf("target port = %d, want 9222", loaded.Upstream.Port)
	}
}

func TestMigrateLegacyConfigRejectsInvalidFile(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, FileName)
	target := filepath.Join(dir, "system", FileName)
	if err := os.WriteFile(legacy, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyConfig(target, legacy); err == nil {
		t.Fatal("expected invalid legacy config to fail")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("invalid legacy config should be preserved: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("invalid source should not create target: %v", err)
	}
}
