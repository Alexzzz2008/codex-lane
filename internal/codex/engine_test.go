package codex

import "testing"

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
