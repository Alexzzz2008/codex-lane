package selfupdate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	predecessorEnv = "CODEXLANE_REPLACES"
	startedEnv     = "CODEXLANE_STARTED"
)

type Replacement struct {
	cmd    *exec.Cmd
	exited chan error
	once   sync.Once
}

func MarkerPath() string {
	return fmt.Sprintf("%s/codexlane-started-%d", os.TempDir(), os.Getpid())
}

func StartReplacement(executable, marker string) (*Replacement, error) {
	_ = os.Remove(marker)
	command := exec.Command(executable)
	command.Env = append(os.Environ(),
		predecessorEnv+"="+strconv.Itoa(os.Getpid()),
		startedEnv+"="+marker,
	)
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	detach(command)
	if err := command.Start(); err != nil {
		return nil, err
	}
	replacement := &Replacement{cmd: command, exited: make(chan error, 1)}
	go func() {
		replacement.exited <- command.Wait()
		close(replacement.exited)
	}()
	return replacement, nil
}

func (r *Replacement) WaitStarted(marker string, timeout time.Duration) error {
	timer := time.NewTicker(50 * time.Millisecond)
	defer timer.Stop()
	deadline := time.After(timeout)
	for {
		if _, err := os.Stat(marker); err == nil {
			return nil
		}
		select {
		case err := <-r.exited:
			if _, statErr := os.Stat(marker); statErr == nil {
				return nil
			}
			if err != nil {
				return fmt.Errorf("new version exited during startup: %w", err)
			}
			return errors.New("new version exited during startup")
		case <-deadline:
			r.Kill()
			return fmt.Errorf("new version did not report readiness within %s", timeout)
		case <-timer.C:
		}
	}
}

func (r *Replacement) Kill() {
	if r == nil || r.cmd == nil || r.cmd.Process == nil {
		return
	}
	r.once.Do(func() {
		_ = r.cmd.Process.Kill()
	})
}

func ReportStarted() {
	marker := strings.TrimSpace(os.Getenv(startedEnv))
	if marker == "" {
		return
	}
	_ = os.Unsetenv(startedEnv)
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return
	}
}

func WaitForPredecessor() {
	value := strings.TrimSpace(os.Getenv(predecessorEnv))
	_ = os.Unsetenv(predecessorEnv)
	pid, err := strconv.Atoi(value)
	if err != nil || pid <= 0 {
		return
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && processAlive(pid) {
		time.Sleep(100 * time.Millisecond)
	}
}
