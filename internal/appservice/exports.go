package appservice

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

type harLog struct {
	Version string        `json:"version"`
	Creator harCreator    `json:"creator"`
	Entries []harEntryRaw `json:"entries"`
}

type harCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harEntryRaw struct {
	StartedDateTime string      `json:"startedDateTime"`
	Time            int64       `json:"time"`
	Request         harRequest  `json:"request"`
	Response        harResponse `json:"response"`
	Lane            harLaneInfo `json:"_lane"`
}

type harRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

type harResponse struct {
	Status      int    `json:"status"`
	StatusText  string `json:"statusText"`
	Redirection bool   `json:"_redirection"`
	Comment     string `json:"comment,omitempty"`
}

type harLaneInfo struct {
	Source    string `json:"source,omitempty"`
	Upstream  string `json:"upstream,omitempty"`
	Process   string `json:"process,omitempty"`
	Result    string `json:"result,omitempty"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
	BytesUp   int64  `json:"bytesUp,omitempty"`
	BytesDown int64  `json:"bytesDown,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *AppService) ExportHAR() (string, error) {
	s.mu.RLock()
	store := s.store
	s.mu.RUnlock()
	if store == nil {
		return "", fmt.Errorf("request store is not ready")
	}
	records := store.List(0)
	log := harLog{
		Version: "1.2",
		Creator: harCreator{Name: "Codex Lane", Version: Version},
		Entries: make([]harEntryRaw, 0, len(records)),
	}
	for _, record := range records {
		entry := harEntryRaw{
			StartedDateTime: record.StartedAt,
			Time:            record.DurationMs,
			Request: harRequest{
				Method: record.Method,
				URL:    fmt.Sprintf("%s://%s:%d", record.Scheme, record.Host, record.Port),
			},
			Response: harResponse{Status: record.Status},
			Lane: harLaneInfo{
				Source:    record.Source,
				Upstream:  record.Upstream,
				Process:   record.Process,
				Result:    record.Result,
				LatencyMs: record.LatencyMs,
				BytesUp:   record.BytesUp,
				BytesDown: record.BytesDown,
				Error:     record.Error,
			},
		}
		if entry.StartedDateTime == "" {
			entry.StartedDateTime = record.FinishedAt
		}
		if record.Result == "failed" {
			entry.Response.Comment = record.Error
		}
		log.Entries = append(log.Entries, entry)
	}
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *AppService) History(hours int) ([]model.HistoryPoint, error) {
	s.mu.RLock()
	logDir := s.paths.LogDirectory
	s.mu.RUnlock()
	if hours <= 0 {
		hours = 24
	}
	if hours > 24*30 {
		hours = 24 * 30
	}
	now := time.Now().Truncate(time.Hour)
	start := now.Add(-time.Duration(hours-1) * time.Hour)
	records, err := readHistoryRecords(logDir, start)
	if err != nil {
		return nil, err
	}
	aggregate := make(map[time.Time]*model.HistoryPoint, hours)
	latencySum := make(map[time.Time]int64, hours)
	latencyCount := make(map[time.Time]int64, hours)
	for index := 0; index < hours; index++ {
		hour := start.Add(time.Duration(index) * time.Hour)
		aggregate[hour] = &model.HistoryPoint{Time: hour.Format("01-02 15:00")}
	}
	for _, record := range records {
		if record.StartedAt == "" {
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, record.StartedAt)
		if err != nil || started.Before(start) {
			continue
		}
		hour := started.Truncate(time.Hour)
		point, ok := aggregate[hour]
		if !ok {
			continue
		}
		point.Requests++
		point.BytesUp += record.BytesUp
		point.BytesDown += record.BytesDown
		if latency := recordLatencyValue(record); latency > 0 || (record.Method != "" && record.Status > 0) {
			latencySum[hour] += latency
			latencyCount[hour]++
		}
	}
	out := make([]model.HistoryPoint, 0, hours)
	for index := 0; index < hours; index++ {
		hour := start.Add(time.Duration(index) * time.Hour)
		point := aggregate[hour]
		if count := latencyCount[hour]; count > 0 {
			point.LatencyMs = latencySum[hour] / count
		}
		out = append(out, *point)
	}
	return out, nil
}

func recordLatencyValue(record model.RequestRecord) int64 {
	if record.LatencyMs > 0 || record.Method != "" {
		return record.LatencyMs
	}
	return record.DurationMs
}

func readHistoryRecords(logDir string, start time.Time) ([]model.RequestRecord, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []model.RequestRecord
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "requests-") || !strings.Contains(name, ".jsonl") {
			continue
		}
		records = append(records, readHistoryFile(filepath.Join(logDir, name), start)...)
	}
	return records, nil
}

func readHistoryFile(path string, start time.Time) []model.RequestRecord {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var records []model.RequestRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record model.RequestRecord
		if json.Unmarshal(line, &record) != nil {
			continue
		}
		if record.StartedAt == "" {
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, record.StartedAt)
		if err != nil || started.Before(start) {
			continue
		}
		records = append(records, record)
	}
	return records
}
