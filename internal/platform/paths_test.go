package platform

import (
	"path/filepath"
	"testing"
)

func TestResolvePathsUsesLocalAppData(t *testing.T) {
	localAppData := t.TempDir()
	t.Setenv("LOCALAPPDATA", localAppData)
	t.Setenv("CODEXLANE_HOME", "")

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	wantConfig := filepath.Join(localAppData, "CodexLane", "launcher.json")
	if paths.ConfigPath != wantConfig {
		t.Fatalf("ConfigPath = %q, want %q", paths.ConfigPath, wantConfig)
	}
	wantLogs := filepath.Join(localAppData, "CodexLane", "logs")
	if paths.LogDirectory != wantLogs {
		t.Fatalf("LogDirectory = %q, want %q", paths.LogDirectory, wantLogs)
	}
}
