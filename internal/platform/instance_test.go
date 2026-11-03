package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstanceRoundTrip(t *testing.T) {
	t.Setenv("CODEXLANE_HOME", t.TempDir())

	if _, ok := ReadInstance(); ok {
		t.Fatal("expected no instance record in a fresh directory")
	}
	if err := WriteInstance("1.2.3"); err != nil {
		t.Fatalf("write instance: %v", err)
	}
	record, ok := ReadInstance()
	if !ok {
		t.Fatal("instance record not found after write")
	}
	if record.Version != "1.2.3" {
		t.Fatalf("version = %q, want 1.2.3", record.Version)
	}
	if record.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", record.PID, os.Getpid())
	}

	ClearInstance()
	if _, ok := ReadInstance(); ok {
		t.Fatal("instance record was not cleared")
	}
}

func TestClearInstanceKeepsForeignRecord(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("CODEXLANE_HOME", directory)

	path := filepath.Join(directory, "instance.json")
	if err := os.WriteFile(path, []byte(`{"pid": 1, "version": "0.0.1"}`), 0o600); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	ClearInstance()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("foreign instance record was removed: %v", err)
	}
}
