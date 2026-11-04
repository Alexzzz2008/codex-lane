package install

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestManagedUsesLocalAppData(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	target := filepath.Join(base, "Programs", "CodexLane")

	if !Managed(filepath.Join(target, "CodexLane.exe")) {
		t.Fatalf("expected %s to be managed", target)
	}
	if Managed(filepath.Join(base, "CodexLane.exe")) {
		t.Fatal("a build outside Programs/CodexLane must not be managed")
	}
}

func TestTargetExecutable(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	target, err := TargetExecutable()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "Programs", "CodexLane", "CodexLane.exe")
	if target != want {
		t.Fatalf("target = %s, want %s", target, want)
	}
}

func TestPortableLocation(t *testing.T) {
	home := "/home/codexlane-test"
	if runtime.GOOS == "windows" {
		home = `C:\Users\codexlane-test`
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	desktop := filepath.Join(home, "Desktop", "CodexLane.exe")
	if !PortableLocation(desktop) {
		t.Fatalf("expected %s to be treated as portable", desktop)
	}
	temp := filepath.Join(os.TempDir(), "CodexLane.exe")
	if !PortableLocation(temp) {
		t.Fatalf("expected %s to be treated as portable", temp)
	}
	project := filepath.Join(home, "Documents", "Codex", "codex-lane", "bin", "CodexLane.exe")
	if PortableLocation(project) {
		t.Fatalf("expected %s to stay untouched", project)
	}
}

func TestMigrationSourceMustMatchInstalledExecutable(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "CodexLane.exe")
	installed := filepath.Join(root, "installed.exe")
	for _, path := range []string{source, installed} {
		if err := os.WriteFile(path, []byte("same-build"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if !removableSource(source, installed) {
		t.Fatal("verified migration source should be removable")
	}
	if removableSource(installed, installed) {
		t.Fatal("must never remove the running executable")
	}
	if err := os.WriteFile(source, []byte("other-file"), 0o700); err != nil {
		t.Fatal(err)
	}
	if removableSource(source, installed) {
		t.Fatal("must preserve a source whose contents changed")
	}
}
