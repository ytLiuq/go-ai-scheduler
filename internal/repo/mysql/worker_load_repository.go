package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

// WorkerLoadRepository persists worker load snapshots.
type WorkerLoadRepository struct {
	db *sql.DB
}

// NewWorkerLoadRepository creates a WorkerLoadRepository.
func NewWorkerLoadRepository(db *sql.DB) *WorkerLoadRepository {
	return &WorkerLoadRepository{db: db}
}

// CreateSnapshot inserts a worker load snapshot.
func (r *WorkerLoadRepository) CreateSnapshot(ctx context.Context, snapshot *model.WorkerLoadSnapshot) error {
	const query = `
		INSERT INTO worker_load_snapshot (worker_id, current_load, max_concurrency, status)
		VALUES (?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		snapshot.WorkerID, snapshot.CurrentLoad, snapshot.MaxConcurrency, snapshot.Status,
	)
	if err != nil {
		return fmt.Errorf("insert worker load snapshot: %w", err)
	}
	return nil
}

// ListSnapshots returns load snapshots for a worker (or all if empty) within a time range.
func (r *WorkerLoadRepository) ListSnapshots(ctx context.Context, workerID string, from, to time.Time, limit int) ([]*model.WorkerLoadSnapshot, error) {
	var query string
	var args []interface{}
	if workerID != "" {
		query = `SELECT id, worker_id, current_load, max_concurrency, status, recorded_at
			FROM worker_load_snapshot
			WHERE worker_id = ? AND recorded_at >= ? AND recorded_at <= ?
			ORDER BY recorded_at DESC`
		args = []interface{}{workerID, from, to}
	} else {
		query = `SELECT id, worker_id, current_load, max_concurrency, status, recorded_at
			FROM worker_load_snapshot
			WHERE recorded_at >= ? AND recorded_at <= ?
			ORDER BY recorded_at DESC`
		args = []interface{}{from, to}
	}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list worker load snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*model.WorkerLoadSnapshot
	for rows.Next() {
		var s model.WorkerLoadSnapshot
		if err := rows.Scan(&s.ID, &s.WorkerID, &s.CurrentLoad, &s.MaxConcurrency, &s.Status, &s.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan worker load snapshot: %w", err)
		}
		snapshots = append(snapshots, &s)
	}
	return snapshots, rows.Err()
}

// DeleteSnapshotsBefore removes snapshots older than the given time.
func (r *WorkerLoadRepository) DeleteSnapshotsBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM worker_load_snapshot WHERE recorded_at < ?`, before,
	)
	if err != nil {
		return 0, fmt.Errorf("delete old worker load snapshots: %w", err)
	}
	return result.RowsAffected()
}
