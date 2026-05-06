package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkerRepository persists worker metadata in MySQL.
type WorkerRepository struct {
	db *gorm.DB
}

// NewWorkerRepository creates a WorkerRepository.
func NewWorkerRepository(db *gorm.DB) *WorkerRepository {
	return &WorkerRepository{db: db}
}

// UpsertWorker inserts or updates a worker row.
func (r *WorkerRepository) UpsertWorker(ctx context.Context, worker *model.WorkerNode) error {
	row := workerNodeRow{
		ID:              worker.ID,
		Hostname:        worker.Hostname,
		IP:              worker.IP,
		CallbackURL:     worker.CallbackURL,
		GRPCAddr:        worker.GRPCAddr,
		Protocol:        worker.Protocol,
		Status:          worker.Status,
		Labels:          worker.Labels,
		MaxConcurrency:  worker.MaxConcurrency,
		CurrentLoad:     worker.CurrentLoad,
		LastHeartbeatAt: worker.LastHeartbeatAt,
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"hostname", "ip", "callback_url", "grpc_addr", "protocol", "status", "labels", "max_concurrency", "current_load", "last_heartbeat_at", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("upsert worker: %w", err)
	}
	return nil
}

// GetWorker loads one worker by id.
func (r *WorkerRepository) GetWorker(ctx context.Context, id string) (*model.WorkerNode, error) {
	var row workerNodeRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("get worker: %w", err)
	}
	return rowToWorker(&row), nil
}

// ListWorkers returns all workers.
func (r *WorkerRepository) ListWorkers(ctx context.Context) ([]*model.WorkerNode, error) {
	var rows []workerNodeRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	return mapWorkers(rows), nil
}

// ListStaleWorkers returns online workers whose last heartbeat is before cutoff.
func (r *WorkerRepository) ListStaleWorkers(ctx context.Context, cutoff time.Time) ([]*model.WorkerNode, error) {
	var rows []workerNodeRow
	if err := r.db.WithContext(ctx).Where("status = ? AND last_heartbeat_at < ?", "online", cutoff).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list stale workers: %w", err)
	}
	return mapWorkers(rows), nil
}

// ListAvailableWorkers returns online workers with remaining capacity.
func (r *WorkerRepository) ListAvailableWorkers(ctx context.Context) ([]*model.WorkerNode, error) {
	var rows []workerNodeRow
	if err := r.db.WithContext(ctx).Where("status = ? AND current_load < max_concurrency", "online").Order("current_load ASC, last_heartbeat_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list available workers: %w", err)
	}
	return mapWorkers(rows), nil
}

func mapWorkers(rows []workerNodeRow) []*model.WorkerNode {
	workers := make([]*model.WorkerNode, 0, len(rows))
	for i := range rows {
		workers = append(workers, rowToWorker(&rows[i]))
	}
	return workers
}
