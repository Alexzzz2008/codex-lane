package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

const desktopEngineSetting = "runCodexInWindowsSubsystemForLinux"

var engineLinePattern = regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(desktopEngineSetting) + `\s*=\s*)(true|false)(\s*(?:#.*)?)$`)

type EngineState struct {
	WSL        bool
	Present    bool
	SourceLine int
}

func ReadEngine(path string) (EngineState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EngineState{}, fmt.Errorf("read Codex config: %w", err)
	}
	return parseEngine(string(data))
}

func parseEngine(content string) (EngineState, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	inDesktop := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inDesktop = trimmed == "[desktop]"
			continue
		}
		if !inDesktop {
			continue
		}
		match := engineLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		value, err := strconv.ParseBool(match[2])
		if err != nil {
			return EngineState{}, fmt.Errorf("parse %s at line %d: %w", desktopEngineSetting, i+1, err)
		}
		return EngineState{WSL: value, Present: true, SourceLine: i + 1}, nil
	}
	return EngineState{WSL: false, Present: false}, nil
}

func ResolveEngine(cfg model.Config, configPath string) (model.ResolvedEngine, error) {
	result := model.ResolvedEngine{
		Mode:       cfg.Engine,
		ConfigPath: configPath,
	}
	state, err := ReadEngine(configPath)
	if err != nil {
		result.Error = err.Error()
		if cfg.Engine == model.EngineAuto {
			return result, err
		}
	} else {
		result.KeyValue = state.WSL
		result.KeyPresent = state.Present
		result.Source = desktopEngineSetting
	}
	switch cfg.Engine {
	case model.EngineAuto:
		if state.WSL {
			result.Resolved = model.EngineWSL
		} else {
			result.Resolved = model.EngineWindows
		}
	case model.EngineWindows, model.EngineWSL:
		result.Resolved = cfg.Engine
	default:
		return result, fmt.Errorf("unsupported engine mode %q", cfg.Engine)
	}
	return result, nil
}

func SetEngine(path string, wsl bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Codex config: %w", err)
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	state, err := parseEngine(content)
	if err != nil {
		return "", err
	}
	value := strconv.FormatBool(wsl)
	lines := strings.Split(content, "\n")
	if state.Present {
		lines[state.SourceLine-1] = engineLinePattern.ReplaceAllString(lines[state.SourceLine-1], "${1}"+value+"${3}")
	} else {
		desktopIndex := -1
		for i, line := range lines {
			if strings.TrimSpace(line) == "[desktop]" {
				desktopIndex = i
				break
			}
		}
		if desktopIndex < 0 {
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
				lines = append(lines, "")
			}
			lines = append(lines, "[desktop]", desktopEngineSetting+" = "+value)
		} else {
			insertAt := desktopIndex + 1
			for insertAt < len(lines) {
				trimmed := strings.TrimSpace(lines[insertAt])
				if strings.HasPrefix(trimmed, "[") {
					break
				}
				insertAt++
			}
			lines = append(lines[:insertAt], append([]string{desktopEngineSetting + " = " + value}, lines[insertAt:]...)...)
		}
	}
	next := strings.Join(lines, "\n")
	verify, err := parseEngine(next)
	if err != nil || !verify.Present || verify.WSL != wsl {
		return "", errors.New("engine setting verification failed")
	}
	backupPath := path + ".bak-" + time.Now().Format("20060102-150405")
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return "", fmt.Errorf("backup Codex config: %w", err)
	}
	if err := atomicWrite(path, []byte(next), 0o600); err != nil {
		return backupPath, err
	}
	return backupPath, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary Codex config: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace Codex config: %w", err)
	}
	return nil
}
