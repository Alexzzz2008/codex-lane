//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessIDsFindsSelf(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	found := ProcessIDs(filepath.Base(executable))
	if _, ok := found[os.Getpid()]; !ok {
		t.Fatalf("current process %d missing from %v", os.Getpid(), found)
	}
	if key := PIDSetKey(found); key == "" {
		t.Fatal("PIDSetKey returned an empty key for a non-empty set")
	}
}

func TestProcessIDsStaysCheap(t *testing.T) {
	start := time.Now()
	for i := 0; i < 50; i++ {
		ProcessIDs("ChatGPT.exe", "Codex.exe")
	}
	elapsed := time.Since(start)
	// A PowerShell/WMI query costs hundreds of milliseconds; anything in the
	// tens of milliseconds keeps the background watcher negligible.
	if average := elapsed / 50; average > 80*time.Millisecond {
		t.Fatalf("snapshot too slow: %s per call", average)
	}
	t.Logf("average %s per snapshot", elapsed/50)
}

func TestPIDSetKeyIsStable(t *testing.T) {
	ids := map[int]string{9: "a", 3: "b", 12: "c"}
	if first, second := PIDSetKey(ids), PIDSetKey(ids); first != second {
		t.Fatalf("unstable key: %q vs %q", first, second)
	}
	if PIDSetKey(nil) != "" {
		t.Fatal("empty set should produce an empty key")
	}
}
