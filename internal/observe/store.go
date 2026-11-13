package observe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

var maxLogSize = int64(32) << 20

type Store struct {
	mu      sync.RWMutex
	records []model.RequestRecord
	start   int
	count   int
	byID    map[string]int
	maxRows int
	active  map[string]requestProgress

	logMu   sync.Mutex
	logDir  string
	logPath string
	logFile *os.File
	logDate string
	logSize int64

	notify    func(event string, record model.RequestRecord)
	total     int64
	success   int64
	failed    int64
	bytesUp   int64
	bytesDown int64
	buckets   [60]bucket

	latencies []int64
	latSorted []int64
	latDirty  bool
}

type bucket struct {
	start time.Time
	down  int64
	lat   int64
	count int64
}

type requestProgress struct {
	up, down int64
	result   int
	latency  bool
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
		active:  make(map[string]requestProgress),
		records: make([]model.RequestRecord, maxRows),
		logDir:  logDir,
	}
	if logDir != "" {
		_ = os.MkdirAll(logDir, 0o700)
	}
	return s
}

func (s *Store) Resize(maxRows int) {
	if maxRows < 100 || maxRows > 100000 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxRows == s.maxRows {
		return
	}
	count := min(s.count, maxRows)
	records := make([]model.RequestRecord, maxRows)
	byID := make(map[string]int, count)
	for index := 0; index < count; index++ {
		record := s.records[(s.start+s.count-count+index)%s.maxRows]
		records[index] = record
		byID[record.ID] = index
	}
	s.records, s.byID, s.maxRows = records, byID, maxRows
	s.start, s.count = 0, count
	if len(s.latencies) > maxRows {
		s.latencies = append([]int64(nil), s.latencies[len(s.latencies)-maxRows:]...)
		s.latDirty = true
	}
}

func (s *Store) Add(record model.RequestRecord) {
	if record.ID == "" {
		record.ID = fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	if record.Time == "" {
		record.Time = time.Now().Format("15:04:05.000")
	}
	s.mu.Lock()
	if s.count == s.maxRows {
		oldest := s.records[s.start%s.maxRows]
		delete(s.byID, oldest.ID)
		s.start++
		s.count--
	}
	position := (s.start + s.count) % s.maxRows
	s.records[position] = record
	s.byID[record.ID] = s.start + s.count
	s.count++
	s.account(record)
	s.mu.Unlock()
	if s.notify != nil {
		s.notify("request:new", record)
	}
}

func (s *Store) Touch(record model.RequestRecord) {
	s.mu.Lock()
	if idx, ok := s.byID[record.ID]; ok {
		s.records[idx%s.maxRows] = record
	}
	s.account(record)
	s.mu.Unlock()
	if s.notify != nil {
		s.notify("request:update", record)
	}
}

func (s *Store) Finish(record model.RequestRecord) {
	s.mu.Lock()
	if idx, ok := s.byID[record.ID]; ok {
		s.records[idx%s.maxRows] = record
	}
	if record.Result == "" && record.Status == 0 {
		record.Result = "failed"
	}
	s.account(record)
	delete(s.active, record.ID)
	s.mu.Unlock()
	if s.logDir != "" {
		s.append(record)
	}
	if s.notify != nil {
		s.notify("request:update", record)
	}
}

func (s *Store) account(record model.RequestRecord) {
	previous, exists := s.active[record.ID]
	if !exists {
		s.total++
	}
	up, down := max(int64(0), record.BytesUp-previous.up), max(int64(0), record.BytesDown-previous.down)
	s.bytesUp += up
	s.bytesDown += down
	b := s.bucketAt(time.Now().Truncate(time.Minute))
	b.down += down
	result := 0
	if record.Result == "failed" || record.Result == "blocked" || record.Status >= 400 {
		result = -1
	} else if record.Result == "ok" || (record.Status >= 200 && record.Status < 400) {
		result = 1
	}
	if result != previous.result {
		if previous.result == 1 {
			s.success--
		}
		if previous.result == -1 {
			s.failed--
		}
		if result == 1 {
			s.success++
		}
		if result == -1 {
			s.failed++
		}
	}
	if latency := recordLatency(record); (latency > 0 || (record.Method != "" && record.Status > 0)) && !previous.latency {
		b.lat += latency
		b.count++
		s.latencies = append(s.latencies, latency)
		if len(s.latencies) > s.maxRows {
			s.latencies = s.latencies[1:]
		}
		s.latDirty, previous.latency = true, true
	}
	s.active[record.ID] = requestProgress{up: record.BytesUp, down: record.BytesDown, result: result, latency: previous.latency}
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
	data = append(data, '\n')
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if err := s.ensureLogFileLocked(); err != nil {
		return
	}
	if s.logSize > 0 && s.logSize+int64(len(data)) > maxLogSize {
		s.rotateLogFileLocked()
		if err := s.ensureLogFileLocked(); err != nil {
			return
		}
	}
	written, err := s.logFile.Write(data)
	if err == nil {
		s.logSize += int64(written)
	}
}

func (s *Store) ensureLogFileLocked() error {
	today := time.Now().Format("20060102")
	if s.logFile != nil && s.logDate == today {
		return nil
	}
	if s.logFile != nil {
		_ = s.logFile.Close()
		s.logFile = nil
	}
	if err := os.MkdirAll(s.logDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(s.logDir, "requests-"+today+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	size := int64(0)
	if info, err := file.Stat(); err == nil {
		size = info.Size()
	}
	s.logFile = file
	s.logDate = today
	s.logPath = path
	s.logSize = size
	return nil
}

func (s *Store) rotateLogFileLocked() {
	_ = s.logFile.Close()
	s.logFile = nil
	rotated := s.logPath + ".1"
	_ = os.Remove(rotated)
	_ = os.Rename(s.logPath, rotated)
	s.logSize = 0
}

func (s *Store) RemoveExpired(days int) int {
	if days <= 0 || s.logDir == "" {
		return 0
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	entries, err := os.ReadDir(s.logDir)
	if err != nil {
		return 0
	}
	s.logMu.Lock()
	current := filepath.Base(s.logPath)
	s.logMu.Unlock()
	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "requests-") || !strings.Contains(name, ".jsonl") {
			continue
		}
		if name == current {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(s.logDir, name)) == nil {
			removed++
		}
	}
	return removed
}

func (s *Store) Close() {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if s.logFile != nil {
		_ = s.logFile.Close()
		s.logFile = nil
	}
}

func (s *Store) List(limit int) []model.RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > s.count {
		limit = s.count
	}
	out := make([]model.RequestRecord, limit)
	for index := 0; index < limit; index++ {
		absolute := s.start + s.count - 1 - index
		out[index] = s.records[absolute%s.maxRows]
	}
	return out
}

func (s *Store) Stats() model.RequestStats {
	s.mu.Lock()
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
	if s.latDirty || s.latSorted == nil {
		sorted := make([]int64, len(s.latencies))
		copy(sorted, s.latencies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		s.latSorted = sorted
		s.latDirty = false
	}
	sorted := s.latSorted
	s.mu.Unlock()
	if len(sorted) > 0 {
		stats.MedianMs = percentile(sorted, 0.5)
		stats.P95Ms = percentile(sorted, 0.95)
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
		if b.start.Equal(minute) {
			if b.count > 0 {
				lat = b.lat / b.count
			}
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
	s.start = 0
	s.count = 0
	s.byID = make(map[string]int)
	s.latencies = nil
	s.latSorted = nil
	s.latDirty = false
	s.total = int64(len(s.active))
	for id, progress := range s.active {
		progress.result = 0
		progress.latency = false
		s.active[id] = progress
	}
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
	if record.LatencyMs > 0 || record.Method != "" {
		return record.LatencyMs
	}
	return record.DurationMs
}
