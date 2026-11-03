package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Instance records the process that currently owns the desktop window, so a
// second launch can tell whether it is older or newer than the running one.
type Instance struct {
	PID        int    `json:"pid"`
	Version    string `json:"version"`
	Executable string `json:"executable"`
	StartedAt  string `json:"startedAt"`
}

// InstancePath returns the location of the running-instance record.
func InstancePath() string {
	return filepath.Join(instanceDataDir(), "instance.json")
}

func instanceDataDir() string {
	if override := strings.TrimSpace(os.Getenv("CODEXLANE_HOME")); override != "" {
		return override
	}
	if base := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); base != "" {
		return filepath.Join(base, "CodexLane")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", "CodexLane")
}

// WriteInstance records the current process as the running instance.
func WriteInstance(version string) error {
	executable, _ := os.Executable()
	record := Instance{
		PID:        os.Getpid(),
		Version:    strings.TrimSpace(version),
		Executable: executable,
		StartedAt:  time.Now().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	path := InstancePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ReadInstance loads the running-instance record. It returns false when the
// record is missing or unreadable.
func ReadInstance() (Instance, bool) {
	data, err := os.ReadFile(InstancePath())
	if err != nil {
		return Instance{}, false
	}
	var record Instance
	if err := json.Unmarshal(data, &record); err != nil {
		return Instance{}, false
	}
	if record.PID <= 0 {
		return Instance{}, false
	}
	return record, true
}

// ClearInstance removes the record only when it still belongs to this process.
func ClearInstance() {
	record, ok := ReadInstance()
	if !ok || record.PID != os.Getpid() {
		return
	}
	_ = os.Remove(InstancePath())
}
