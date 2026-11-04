//go:build windows

package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGuardProcessHelper(t *testing.T) {
	if os.Getenv("CODEXLANE_TEST_HELPER") != "wait" {
		return
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestGuardKillsUnreadyProcessAndRestoresBackup(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "CodexLane.exe")
	backup := executable + ".old"
	if err := os.WriteFile(executable, []byte("failed update"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("working version"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeRecovery(executable, backup, 0); err != nil {
		t.Fatal(err)
	}
	testExecutable, _ := os.Executable()
	command := exec.Command(testExecutable, "-test.run=^TestGuardProcessHelper$")
	command.Env = append(os.Environ(), "CODEXLANE_TEST_HELPER=wait")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	defer command.Wait()
	// The fixture is deliberately not executable: restoration must still happen
	// and report that the recovered program could not be launched.
	if err := GuardUpdate(executable, command.Process.Pid, 20*time.Millisecond); err == nil {
		t.Fatal("expected fixture launch error")
	}
	if processAlive(command.Process.Pid) {
		t.Fatal("unready update process was not stopped")
	}
	restored, err := os.ReadFile(executable)
	if err != nil || string(restored) != "working version" {
		t.Fatalf("backup was not restored: %q, %v", restored, err)
	}
	if _, err := os.Stat(executable + ".update.json"); !os.IsNotExist(err) {
		t.Fatal("recovery record was not cleared")
	}
}
