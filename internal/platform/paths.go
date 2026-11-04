package platform

import (
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	ExecutableDir string
	ConfigPath    string
	LogDirectory  string
	UserHome      string
	CodexHome     string
	LocalAppData  string
}

func ResolvePaths() (Paths, error) {
	exe, err := os.Executable()
	if err != nil {
		return Paths{}, err
	}
	exeDir := filepath.Dir(exe)
	home, _ := os.UserHomeDir()
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	configBase := filepath.Join(localAppData, "CodexLane")
	if override := strings.TrimSpace(os.Getenv("CODEXLANE_HOME")); override != "" {
		configBase = override
	}
	paths := Paths{
		ExecutableDir: exeDir,
		ConfigPath:    filepath.Join(configBase, "launcher.json"),
		LogDirectory:  filepath.Join(configBase, "logs"),
		UserHome:      home,
		CodexHome:     codexHome,
		LocalAppData:  localAppData,
	}
	if err := os.MkdirAll(filepath.Dir(paths.ConfigPath), 0o700); err != nil {
		return paths, err
	}
	if err := os.MkdirAll(paths.LogDirectory, 0o700); err != nil {
		return paths, err
	}
	return paths, nil
}
