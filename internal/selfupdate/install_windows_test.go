//go:build windows

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInstallClearsHiddenAttribute(t *testing.T) {
	previous := canStart
	canStart = func(string) error { return nil }
	t.Cleanup(func() { canStart = previous })

	directory := t.TempDir()
	executable := filepath.Join(directory, "CodexLane.exe")
	staged := filepath.Join(directory, "staged.exe")
	if err := os.WriteFile(executable, []byte("old version"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new version"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(staged, executable); err != nil {
		t.Fatalf("install: %v", err)
	}
	name, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := windows.GetFileAttributes(name)
	if err != nil {
		t.Fatalf("attributes: %v", err)
	}
	if attributes&windows.FILE_ATTRIBUTE_HIDDEN != 0 {
		t.Fatal("installed executable kept the hidden attribute")
	}
}
