package appservice

import (
	"os"
	"path/filepath"

	"github.com/Alexzzz2008/codex-lane/internal/install"
	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func installState() model.InstallState {
	executable, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	state := model.InstallState{
		Managed:    install.Managed(executable),
		Executable: executable,
	}
	if target, err := install.TargetExecutable(); err == nil {
		state.InstallPath = target
	}
	if shortcut, err := install.ShortcutPath(); err == nil {
		state.ShortcutPath = shortcut
		if _, err := os.Stat(shortcut); err == nil {
			state.Shortcut = true
		}
	}
	return state
}
