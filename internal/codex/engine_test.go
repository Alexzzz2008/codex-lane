package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestParseEngine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wsl     bool
		present bool
	}{
		{"true", "[desktop]\nrunCodexInWindowsSubsystemForLinux = true\n", true, true},
		{"false", "[desktop]\r\nrunCodexInWindowsSubsystemForLinux = false\r\n", false, true},
		{"comment", "[desktop]\n  runCodexInWindowsSubsystemForLinux = true # current\n", true, true},
		{"missing key", "[desktop]\nfoo = 1\n", false, false},
		{"other section", "[desktop]\nfoo = 1\n[other]\nrunCodexInWindowsSubsystemForLinux = true\n", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := parseEngine(tt.content)
			if err != nil {
				t.Fatal(err)
			}
			if state.WSL != tt.wsl || state.Present != tt.present {
				t.Fatalf("got wsl=%v present=%v", state.WSL, state.Present)
			}
		})
	}
}

func TestMissingConfigDefaultsToWindowsWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.toml")
	engine, err := ResolveEngine(config.Default(), path)
	if err != nil || engine.Resolved != model.EngineWindows || engine.KeyPresent {
		t.Fatalf("missing config: %+v, error: %v", engine, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("auto detection must not create Codex config")
	}
}

func TestManualEngineCreatesMissingConfigAndPreservesExistingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "config.toml")
	backup, err := SetEngine(path, true)
	if err != nil || backup != "" {
		t.Fatalf("create engine setting: backup=%s, error=%v", backup, err)
	}
	state, err := ReadEngine(path)
	if err != nil || !state.Present || !state.WSL {
		t.Fatalf("WSL setting not created: %+v, error=%v", state, err)
	}
	original := "model = \"fixture\"\n[desktop]\nother = 123\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, err = SetEngine(path, false)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(backup)
	if err != nil || string(saved) != original {
		t.Fatal("existing Codex config was not backed up intact")
	}
	state, err = ReadEngine(path)
	if err != nil || !state.Present || state.WSL {
		t.Fatalf("Windows setting not written: %+v, error=%v", state, err)
	}
}
