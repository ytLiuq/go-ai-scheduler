package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"gorm.io/gorm"
)

// WorkerLoadRepository persists worker load snapshots.
type WorkerLoadRepository struct {
	db *gorm.DB
}

// NewWorkerLoadRepository creates a WorkerLoadRepository.
func NewWorkerLoadRepository(db *gorm.DB) *WorkerLoadRepository {
	return &WorkerLoadRepository{db: db}
}

// CreateSnapshot inserts a worker load snapshot.
func (r *WorkerLoadRepository) CreateSnapshot(ctx context.Context, snapshot *model.WorkerLoadSnapshot) error {
	row := workerLoadSnapshotRow{
		WorkerID:       snapshot.WorkerID,
		CurrentLoad:    snapshot.CurrentLoad,
		MaxConcurrency: snapshot.MaxConcurrency,
		Status:         snapshot.Status,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert worker load snapshot: %w", err)
	}
	snapshot.ID = row.ID
	snapshot.RecordedAt = row.RecordedAt
	return nil
}

// ListSnapshots returns load snapshots for a worker (or all if empty) within a time range.
func (r *WorkerLoadRepository) ListSnapshots(ctx context.Context, workerID string, from, to time.Time, limit int) ([]*model.WorkerLoadSnapshot, error) {
	query := r.db.WithContext(ctx).Where("recorded_at >= ? AND recorded_at <= ?", from, to).Order("recorded_at DESC")
	if workerID != "" {
		query = query.Where("worker_id = ?", workerID)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []workerLoadSnapshotRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list worker load snapshots: %w", err)
	}
	snapshots := make([]*model.WorkerLoadSnapshot, 0, len(rows))
	for i := range rows {
		snapshots = append(snapshots, &model.WorkerLoadSnapshot{
			ID:             rows[i].ID,
			WorkerID:       rows[i].WorkerID,
			CurrentLoad:    rows[i].CurrentLoad,
			MaxConcurrency: rows[i].MaxConcurrency,
			Status:         rows[i].Status,
			RecordedAt:     rows[i].RecordedAt,
		})
	}
	return snapshots, nil
}

// DeleteSnapshotsBefore removes snapshots older than the given time.
func (r *WorkerLoadRepository) DeleteSnapshotsBefore(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("recorded_at < ?", before).Delete(&workerLoadSnapshotRow{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete old worker load snapshots: %w", result.Error)
	}
	return result.RowsAffected, nil
}
