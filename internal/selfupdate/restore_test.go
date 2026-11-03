package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestorePrevious(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "CodexLane.exe")
	backup := executable + ".old"
	if err := os.WriteFile(executable, []byte("new version"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("old version"), 0o755); err != nil {
		t.Fatal(err)
	}
	restored, err := RestorePrevious(executable)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored != executable {
		t.Fatalf("restored %q, want %q", restored, executable)
	}
	content, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old version" {
		t.Fatalf("content = %q", content)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup should be gone, stat error = %v", err)
	}
}

func TestRestorePreviousWithoutBackup(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "CodexLane.exe")
	if err := os.WriteFile(executable, []byte("new version"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := RestorePrevious(executable); err == nil {
		t.Fatal("expected an error when no backup exists")
	}
}
