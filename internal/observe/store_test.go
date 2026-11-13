package observe

import (
	"fmt"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestResizePreservesRecentRecordsAndUpdates(t *testing.T) {
	store := NewStore(200, "", nil)
	for index := 0; index < 250; index++ {
		store.Add(model.RequestRecord{ID: fmt.Sprint(index)})
	}
	store.Resize(100)
	rows := store.List(0)
	if len(rows) != 100 || rows[0].ID != "249" || rows[99].ID != "150" {
		t.Fatalf("incorrect retained rows: %v", rows)
	}
	store.Touch(model.RequestRecord{ID: "200", Status: 204})
	if store.List(0)[49].Status != 204 {
		t.Fatal("resized row index was not updated")
	}
	store.Resize(300)
	store.Add(model.RequestRecord{ID: "250"})
	if rows := store.List(0); len(rows) != 101 || rows[0].ID != "250" {
		t.Fatal("growing the store lost rows")
	}
}

func TestActiveConnectionStatsAndTrafficAreImmediateAndNotCountedTwice(t *testing.T) {
	store := NewStore(100, "", nil)
	record := model.RequestRecord{ID: "live"}
	store.Add(record)
	record.Status, record.Result, record.LatencyMs = 200, "ok", 42
	record.BytesUp, record.BytesDown = 10, 20
	store.Touch(record)
	stats := store.Stats()
	if stats.Total != 1 || stats.Success != 1 || stats.BytesDown != 20 || stats.MedianMs != 42 {
		t.Fatalf("active connection is missing: %+v", stats)
	}
	series := store.Series()
	if series[len(series)-1].BytesDown != 20 {
		t.Fatal("active traffic is missing from chart")
	}
	store.Touch(record)
	record.BytesUp, record.BytesDown = 15, 30
	store.Finish(record)
	stats = store.Stats()
	if stats.Total != 1 || stats.Success != 1 || stats.BytesUp != 15 || stats.BytesDown != 30 {
		t.Fatalf("connection was counted twice: %+v", stats)
	}
}

func TestFailedTransferOverridesSuccessfulResponseStatus(t *testing.T) {
	store := NewStore(100, "", nil)
	record := model.RequestRecord{ID: "truncated", Method: "GET"}
	store.Add(record)
	record.Status, record.Result = 200, "ok"
	store.Touch(record)
	record.Result, record.Error = "failed", "response interrupted"
	store.Finish(record)
	if stats := store.Stats(); stats.Total != 1 || stats.Success != 0 || stats.Failed != 1 {
		t.Fatalf("interrupted transfer was counted as successful: %+v", stats)
	}
}

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

func TestZeroMillisecondConnectLatencyDoesNotBecomeTunnelDuration(t *testing.T) {
	store := NewStore(100, "", nil)
	record := model.RequestRecord{ID: "fast-local-proxy", Method: "CONNECT", Status: 200, Result: "ok", DurationMs: 60000}
	store.Add(record)
	store.Touch(record)
	store.Finish(record)
	if stats := store.Stats(); stats.MedianMs != 0 || stats.P95Ms != 0 {
		t.Fatalf("connection lifetime was reported as latency: %+v", stats)
	}
	for _, point := range store.Series() {
		if point.LatencyMs != 0 {
			t.Fatalf("chart contains tunnel duration: %+v", point)
		}
	}
}

func TestZeroLatencySamplesParticipateInMedianAndAverage(t *testing.T) {
	store := NewStore(100, "", nil)
	store.Add(model.RequestRecord{ID: "still-connecting", Method: "CONNECT"})
	for index, latency := range []int64{0, 0, 90} {
		record := model.RequestRecord{ID: fmt.Sprint(index), Method: "CONNECT"}
		store.Add(record)
		record.Status, record.Result, record.LatencyMs = 200, "ok", latency
		store.Touch(record)
		store.Finish(record)
	}
	if stats := store.Stats(); stats.MedianMs != 0 {
		t.Fatalf("zero latency samples were discarded: %+v", stats)
	}
	points := store.Series()
	if points[len(points)-1].LatencyMs != 30 {
		t.Fatalf("expected average of 0, 0, 90 to be 30: %+v", points[len(points)-1])
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
