package observe

import (
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestStatsUseConnectLatencyForLongTunnels(t *testing.T) {
	store := NewStore(1000, t.TempDir(), nil)
	t.Cleanup(store.Close)
	store.Add(model.RequestRecord{ID: "a", LatencyMs: 90, DurationMs: 41000})
	store.Finish(model.RequestRecord{ID: "a", LatencyMs: 90, DurationMs: 41000, Result: "ok"})
	store.Add(model.RequestRecord{ID: "b", LatencyMs: 120, DurationMs: 38000})
	store.Finish(model.RequestRecord{ID: "b", LatencyMs: 120, DurationMs: 38000, Result: "ok"})

	stats := store.Stats()
	if stats.P95Ms > 1000 {
		t.Fatalf("P95 should use connect latency, got %d", stats.P95Ms)
	}
	if stats.MedianMs > 1000 {
		t.Fatalf("median should use connect latency, got %d", stats.MedianMs)
	}

	for _, point := range store.Series() {
		if point.LatencyMs > 1000 {
			t.Fatalf("series latency should use connect latency, got %d", point.LatencyMs)
		}
	}
}

func TestSeriesKeepsRollingHourAfterLongRuntime(t *testing.T) {
	store := NewStore(1000, t.TempDir(), nil)
	t.Cleanup(store.Close)
	store.mu.Lock()
	for index := range store.buckets {
		store.buckets[index] = bucket{start: time.Now().Add(-3 * time.Hour)}
	}
	store.mu.Unlock()

	store.Add(model.RequestRecord{ID: "later", LatencyMs: 42, BytesDown: 2048})
	store.Finish(model.RequestRecord{ID: "later", LatencyMs: 42, BytesDown: 2048, Result: "ok"})

	series := store.Series()
	if len(series) != bucketCount {
		t.Fatalf("series should cover %d minutes, got %d", bucketCount, len(series))
	}
	latest := series[len(series)-1]
	if latest.BytesDown != 2048 || latest.LatencyMs != 42 {
		t.Fatalf("latest bucket lost traffic after long runtime: %+v", latest)
	}
}
