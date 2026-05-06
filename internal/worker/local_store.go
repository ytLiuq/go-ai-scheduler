package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	apiservice "github.com/example/go-ai-scheduler/internal/api/service"
)

// StoredReport is a serialised task status report held for later delivery.
type StoredReport struct {
	SchedulerURL string                            `json:"scheduler_url"`
	Report       apiservice.TaskStatusReportRequest `json:"report"`
	StoredAt     int64                             `json:"stored_at"`
}

// Store buffers task reports that could not be delivered to the scheduler
// and retries delivery when connectivity is restored.
type Store struct {
	mu       sync.Mutex
	dir      string
	reporter *ReportClient
	logger   *slog.Logger
	baseDir  string
}

// NewStore creates a local store backed by the given directory.
func NewStore(baseDir string, reporter *ReportClient, l *slog.Logger) (*Store, error) {
	dir := filepath.Join(baseDir, "worker-local-store")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Store{
		dir:      dir,
		reporter: reporter,
		logger:   l,
		baseDir:  baseDir,
	}, nil
}

// Buffer persists a report that could not be sent to the scheduler.
func (s *Store) Buffer(schedulerURL string, report apiservice.TaskStatusReportRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sr := StoredReport{
		SchedulerURL: schedulerURL,
		Report:       report,
		StoredAt:     time.Now().Unix(),
	}
	data, err := json.Marshal(sr)
	if err != nil {
		s.logger.Warn("localstore: marshal report failed", "schedule_instance_id", report.ScheduleInstanceID, "error", err)
		return
	}

	filename := filepath.Join(s.dir, report.ScheduleInstanceID+".json")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		s.logger.Warn("localstore: buffer report failed", "schedule_instance_id", report.ScheduleInstanceID, "error", err)
		return
	}
	s.logger.Info("localstore: buffered report", "schedule_instance_id", report.ScheduleInstanceID)
}

// Remove deletes a buffered report after successful delivery.
func (s *Store) Remove(scheduleInstanceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filename := filepath.Join(s.dir, scheduleInstanceID+".json")
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("localstore: remove report failed", "schedule_instance_id", scheduleInstanceID, "error", err)
	}
}

// Flush attempts to deliver all buffered reports and removes them on success.
func (s *Store) Flush(ctx context.Context) int {
	s.mu.Lock()
	entries, err := os.ReadDir(s.dir)
	s.mu.Unlock()
	if err != nil {
		s.logger.Warn("localstore: read dir failed", "error", err)
		return -1
	}

	var remaining int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			s.logger.Warn("localstore: read file failed", "file", filename, "error", err)
			remaining++
			continue
		}
		var sr StoredReport
		if err := json.Unmarshal(data, &sr); err != nil {
			s.logger.Warn("localstore: unmarshal failed", "file", filename, "error", err)
			_ = os.Remove(filename)
			continue
		}

		deliveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = s.reporter.Report(deliveryCtx, sr.SchedulerURL, sr.Report)
		cancel()
		if err != nil {
			s.logger.Warn("localstore: flush failed", "schedule_instance_id", sr.Report.ScheduleInstanceID, "error", err)
			remaining++
			continue
		}

		_ = os.Remove(filename)
		s.logger.Info("localstore: flushed report", "schedule_instance_id", sr.Report.ScheduleInstanceID)
	}
	return remaining
}

// PendingCount returns the number of buffered reports awaiting delivery.
func (s *Store) PendingCount() int {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}
	return count
}

// StartFlushLoop periodically attempts to deliver buffered reports.
func (s *Store) StartFlushLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("localstore flush loop stopped")
			return
		case <-ticker.C:
			remaining := s.Flush(ctx)
			if remaining > 0 {
				s.logger.Warn("localstore: reports still pending after flush", "count", remaining)
			}
		}
	}
}
