package selfupdate

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestReadinessRequiresMatchingProcess(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ready")
	if err := os.WriteFile(marker, []byte("-1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := waitReady(marker, os.Getpid(), 10*time.Millisecond); err == nil {
		t.Fatal("accepted another process's marker")
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := waitReady(marker, os.Getpid(), time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackWithoutBackupPreservesInstalledExecutable(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "CodexLane.exe")
	if err := os.WriteFile(executable, []byte("working version"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(executable, executable+".old"); err == nil {
		t.Fatal("expected missing backup error")
	}
	data, err := os.ReadFile(executable)
	if err != nil || string(data) != "working version" {
		t.Fatalf("destroyed current executable: %q, %v", data, err)
	}
}

func TestRecoveryRejectsPathsOutsideApplicationDirectory(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "CodexLane.exe")
	if err := writeRecovery(executable, filepath.Join(t.TempDir(), "unrelated.exe"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := readRecovery(executable); err == nil {
		t.Fatal("accepted unrelated backup path")
	}
}
