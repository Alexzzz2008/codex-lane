//go:build !windows

package platform

import "os/exec"

func configureHiddenProcess(cmd *exec.Cmd) {}

func ConfigureDetached(cmd *exec.Cmd) {}
