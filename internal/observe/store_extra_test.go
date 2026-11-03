package observe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestRingBufferKeepsNewestAndUpdatesAfterWrap(t *testing.T) {
	store := NewStore(100, "", nil)
	for index := 1; index <= 103; index++ {
		store.Add(model.RequestRecord{ID: string(rune('a' + index - 1))})
	}
	list := store.List(10)
	if len(list) != 10 {
		t.Fatalf("len = %d, want 10", len(list))
	}
	if list[0].ID != string(rune('a'+102)) || list[9].ID != string(rune('a'+93)) {
		t.Fatalf("unexpected order: %v", list)
	}
	if _, exists := store.byID["a"]; exists {
		t.Fatalf("evicted record should leave the index")
	}

	store.Touch(model.RequestRecord{ID: "e", Status: 200, Result: "ok"})
	for _, record := range store.List(10) {
		if record.ID == "e" {
			t.Fatalf("evicted record e should not be listed")
		}
	}
	touched := model.RequestRecord{ID: string(rune('a' + 102)), Status: 200, Result: "ok"}
	store.Touch(touched)
	found := false
	for _, record := range store.List(10) {
		if record.ID == touched.ID {
			found = true
			if record.Result != "ok" {
				t.Fatalf("touch after wrap lost the update")
			}
		}
	}
	if !found {
		t.Fatalf("touched record f missing from list")
	}
}

func TestFinishLatencyWindowIsBounded(t *testing.T) {
	store := NewStore(100, "", nil)
	for index := 0; index < 103; index++ {
		record := model.RequestRecord{ID: string(rune('a' + index)), LatencyMs: int64(index + 1)}
		store.Add(record)
		store.Finish(record)
	}
	stats := store.Stats()
	if stats.P95Ms != 98 {
		t.Fatalf("p95 = %d, want 98 (newest 100 latencies 4..103)", stats.P95Ms)
	}
	if stats.MedianMs != 53 {
		t.Fatalf("median = %d, want 53", stats.MedianMs)
	}
}

func TestLogRotationAndRetention(t *testing.T) {
	logDir := t.TempDir()
	previous := maxLogSize
	maxLogSize = 128
	t.Cleanup(func() { maxLogSize = previous })

	store := NewStore(100, logDir, nil)
	for index := 0; index < 6; index++ {
		record := model.RequestRecord{
			ID:        string(rune('a' + index)),
			Host:      "example.com",
			StartedAt: time.Now().Format(time.RFC3339Nano),
		}
		store.Add(record)
		store.Finish(record)
	}
	store.Close()
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected rotated log files, got %d", len(entries))
	}
	rotated := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl.1") {
			rotated = true
		}
	}
	if !rotated {
		t.Fatalf("expected a .jsonl.1 rotated file")
	}

	oldPath := filepath.Join(logDir, "requests-19990101.jsonl")
	if err := os.WriteFile(oldPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	stale := time.Now().AddDate(0, 0, -30)
	if err := os.Chtimes(oldPath, stale, stale); err != nil {
		t.Fatalf("touch old log: %v", err)
	}
	removed := store.RemoveExpired(14)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expired log still exists")
	}
}
