package appservice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestHistoryKeepsZeroConnectLatency(t *testing.T) {
	service := New()
	service.paths.LogDirectory = t.TempDir()
	record := model.RequestRecord{ID: "fast", Method: "CONNECT", StartedAt: time.Now().Format(time.RFC3339Nano), DurationMs: 60000, BytesDown: 120}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(service.paths.LogDirectory, "requests-test.jsonl"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	points, err := service.History(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].Requests != 1 || points[0].BytesDown != 120 || points[0].LatencyMs != 0 {
		t.Fatalf("incorrect history: %+v", points)
	}
}

func TestHistoryIncludesZeroInLatencyAverage(t *testing.T) {
	service := New()
	service.paths.LogDirectory = t.TempDir()
	var data []byte
	for _, latency := range []int64{0, 0, 90} {
		record := model.RequestRecord{Method: "CONNECT", Status: 200, LatencyMs: latency, StartedAt: time.Now().Format(time.RFC3339Nano)}
		row, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, append(row, '\n')...)
	}
	if err := os.WriteFile(filepath.Join(service.paths.LogDirectory, "requests-test.jsonl"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	points, err := service.History(1)
	if err != nil || len(points) != 1 || points[0].Requests != 3 || points[0].LatencyMs != 30 {
		t.Fatalf("incorrect zero-inclusive history average: %+v, %v", points, err)
	}
}
