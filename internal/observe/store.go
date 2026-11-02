package observe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

type Store struct {
	mu        sync.RWMutex
	records   []model.RequestRecord
	byID      map[string]int
	maxRows   int
	logPath   string
	notify    func(event string, record model.RequestRecord)
	total     int64
	success   int64
	failed    int64
	bytesUp   int64
	bytesDown int64
	buckets   [60]bucket
}

type bucket struct {
	start time.Time
	down  int64
	lat   int64
	count int64
}

const bucketCount = 60

func NewStore(maxRows int, logDir string, notify func(event string, record model.RequestRecord)) *Store {
	if maxRows < 100 {
		maxRows = 10000
	}
	s := &Store{
		maxRows: maxRows,
		notify:  notify,
		byID:    make(map[string]int),
	}
	if logDir != "" {
		_ = os.MkdirAll(logDir, 0o700)
		s.logPath = filepath.Join(logDir, "requests-"+time.Now().Format("20060102")+".jsonl")
	}
	return s
}

func (s *Store) Add(record model.RequestRecord) {
	if record.ID == "" {
		record.ID = fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	if record.Time == "" {
		record.Time = time.Now().Format("15:04:05.000")
	}
	s.mu.Lock()
	if len(s.records) >= s.maxRows {
		copy(s.records, s.records[len(s.records)-s.maxRows+1:])
		s.records = s.records[:s.maxRows-1]
		s.byID = make(map[string]int, len(s.records))
		for i := range s.records {
			s.byID[s.records[i].ID] = i
		}
	}
	s.byID[record.ID] = len(s.records)
	s.records = append(s.records, record)
	s.mu.Unlock()
	if s.notify != nil {
		s.notify("request:new", record)
	}
}

func (s *Store) Touch(record model.RequestRecord) {
	s.mu.Lock()
	if idx, ok := s.byID[record.ID]; ok && idx >= 0 && idx < len(s.records) {
		s.records[idx] = record
	}
	s.mu.Unlock()
	if s.notify != nil {
		s.notify("request:update", record)
	}
}

func (s *Store) Finish(record model.RequestRecord) {
	s.mu.Lock()
	idx, ok := s.byID[record.ID]
	if ok && idx >= 0 && idx < len(s.records) {
		s.records[idx] = record
	}
	s.total++
	if record.Result == "ok" || (record.Status >= 200 && record.Status < 400) {
		s.success++
	} else {
		s.failed++
	}
	s.bytesUp += record.BytesUp
	s.bytesDown += record.BytesDown
	minute := time.Now().Truncate(time.Minute)
	b := s.bucketAt(minute)
	b.down += record.BytesDown
	b.lat += recordLatency(record)
	b.count++
	s.mu.Unlock()
	if s.logPath != "" {
		s.append(record)
	}
	if s.notify != nil {
		s.notify("request:update", record)
	}
}

func (s *Store) bucketAt(minute time.Time) *bucket {
	index := int(minute.Unix()/60) % len(s.buckets)
	if index < 0 {
		index += len(s.buckets)
	}
	b := &s.buckets[index]
	if !b.start.Equal(minute) {
		*b = bucket{start: minute}
	}
	return b
}

func (s *Store) append(record model.RequestRecord) {
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	f, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
	_ = f.Close()
}

func (s *Store) List(limit int) []model.RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.records) {
		limit = len(s.records)
	}
	out := make([]model.RequestRecord, limit)
	start := len(s.records) - limit
	for i := 0; i < limit; i++ {
		out[i] = s.records[start+i]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (s *Store) Stats() model.RequestStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := model.RequestStats{
		Total:     s.total,
		Success:   s.success,
		Failed:    s.failed,
		BytesUp:   s.bytesUp,
		BytesDown: s.bytesDown,
	}
	if stats.Total > 0 {
		stats.SuccessRate = float64(stats.Success) / float64(stats.Total) * 100
	}
	latencies := make([]int64, 0, len(s.records))
	for _, record := range s.records {
		if value := recordLatency(record); value > 0 {
			latencies = append(latencies, value)
		}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	if len(latencies) > 0 {
		stats.MedianMs = percentile(latencies, 0.5)
		stats.P95Ms = percentile(latencies, 0.95)
	}
	return stats
}

func (s *Store) Series() []model.RequestSeriesPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().Truncate(time.Minute)
	out := make([]model.RequestSeriesPoint, 0, bucketCount)
	for i := bucketCount - 1; i >= 0; i-- {
		minute := now.Add(-time.Duration(i) * time.Minute)
		index := int(minute.Unix()/60) % len(s.buckets)
		if index < 0 {
			index += len(s.buckets)
		}
		b := s.buckets[index]
		lat := int64(0)
		down := int64(0)
		if b.start.Equal(minute) && b.count > 0 {
			lat = b.lat / b.count
			down = b.down
		}
		out = append(out, model.RequestSeriesPoint{
			Time:      minute.Format("15:04"),
			BytesDown: down,
			LatencyMs: lat,
		})
	}
	return out
}

func (s *Store) Clear() {
	s.mu.Lock()
	s.records = nil
	s.byID = make(map[string]int)
	s.total = 0
	s.success = 0
	s.failed = 0
	s.bytesUp = 0
	s.bytesDown = 0
	s.buckets = [bucketCount]bucket{}
	s.mu.Unlock()
	if s.notify != nil {
		s.notify("request:clear", model.RequestRecord{})
	}
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1) * p)
	return sorted[index]
}

func recordLatency(record model.RequestRecord) int64 {
	if record.LatencyMs > 0 {
		return record.LatencyMs
	}
	return record.DurationMs
}
