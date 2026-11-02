package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type CommandResult struct {
	Stdout string
	Stderr string
	Code   int
}

func Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	configureHiddenProcess(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		result.Code = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func PowerShell(ctx context.Context, script string) (CommandResult, error) {
	prefix := "[Console]::OutputEncoding=[System.Text.Encoding]::UTF8; $ErrorActionPreference='Stop'; "
	return Run(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", prefix+script)
}

func PowerShellJSON(ctx context.Context, script string, target any) error {
	result, err := PowerShell(ctx, "& {"+script+"\n} | ConvertTo-Json -Depth 8 -Compress")
	if err != nil {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = err.Error()
		}
		return errors.New(message)
	}
	output := strings.TrimSpace(strings.TrimPrefix(result.Stdout, "\ufeff"))
	if output == "" {
		return errors.New("PowerShell returned no data")
	}
	if err := json.Unmarshal([]byte(output), target); err != nil {
		return fmt.Errorf("decode PowerShell JSON: %w", err)
	}
	return nil
}

func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return context.WithTimeout(parent, timeout)
}

func StartProcess(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	configureHiddenProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}
