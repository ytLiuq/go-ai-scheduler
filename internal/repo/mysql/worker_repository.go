package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

// WorkerRepository persists worker metadata in MySQL.
type WorkerRepository struct {
	db *sql.DB
}

// NewWorkerRepository creates a WorkerRepository.
func NewWorkerRepository(db *sql.DB) *WorkerRepository {
	return &WorkerRepository{db: db}
}

// UpsertWorker inserts or updates a worker row.
func (r *WorkerRepository) UpsertWorker(ctx context.Context, worker *model.WorkerNode) error {
	const query = `
		INSERT INTO worker_node (
			id, hostname, ip, callback_url, grpc_addr, protocol, status, labels, max_concurrency, current_load, last_heartbeat_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			hostname = VALUES(hostname),
			ip = VALUES(ip),
			callback_url = VALUES(callback_url),
			grpc_addr = VALUES(grpc_addr),
			protocol = VALUES(protocol),
			status = VALUES(status),
			labels = VALUES(labels),
			max_concurrency = VALUES(max_concurrency),
			current_load = VALUES(current_load),
			last_heartbeat_at = VALUES(last_heartbeat_at),
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := r.db.ExecContext(ctx, query,
		worker.ID,
		worker.Hostname,
		worker.IP,
		worker.CallbackURL,
		worker.GRPCAddr,
		worker.Protocol,
		worker.Status,
		worker.Labels,
		worker.MaxConcurrency,
		worker.CurrentLoad,
		worker.LastHeartbeatAt,
	)
	if err != nil {
		return fmt.Errorf("upsert worker: %w", err)
	}
	return nil
}

// GetWorker loads one worker by id.
func (r *WorkerRepository) GetWorker(ctx context.Context, id string) (*model.WorkerNode, error) {
	const query = `
		SELECT id, hostname, ip, callback_url, grpc_addr, protocol, status, labels, max_concurrency, current_load, last_heartbeat_at
		FROM worker_node
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)
	return scanWorker(row)
}

// ListWorkers returns all workers.
func (r *WorkerRepository) ListWorkers(ctx context.Context) ([]*model.WorkerNode, error) {
	const query = `
		SELECT id, hostname, ip, callback_url, grpc_addr, protocol, status, labels, max_concurrency, current_load, last_heartbeat_at
		FROM worker_node
		ORDER BY id
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	defer rows.Close()
	return scanWorkers(rows)
}

// ListStaleWorkers returns online workers whose last heartbeat is before cutoff.
func (r *WorkerRepository) ListStaleWorkers(ctx context.Context, cutoff time.Time) ([]*model.WorkerNode, error) {
	const query = `
		SELECT id, hostname, ip, callback_url, grpc_addr, protocol, status, labels, max_concurrency, current_load, last_heartbeat_at
		FROM worker_node
		WHERE status = 'online' AND last_heartbeat_at < ?
		ORDER BY id
	`
	rows, err := r.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("list stale workers: %w", err)
	}
	defer rows.Close()
	return scanWorkers(rows)
}

// ListAvailableWorkers returns online workers with remaining capacity.
func (r *WorkerRepository) ListAvailableWorkers(ctx context.Context) ([]*model.WorkerNode, error) {
	const query = `
		SELECT id, hostname, ip, callback_url, grpc_addr, protocol, status, labels, max_concurrency, current_load, last_heartbeat_at
		FROM worker_node
		WHERE status = 'online' AND current_load < max_concurrency
		ORDER BY current_load ASC, last_heartbeat_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list available workers: %w", err)
	}
	defer rows.Close()
	return scanWorkers(rows)
}

func scanWorker(scanner interface{ Scan(dest ...any) error }) (*model.WorkerNode, error) {
	var worker model.WorkerNode
	if err := scanner.Scan(
		&worker.ID,
		&worker.Hostname,
		&worker.IP,
		&worker.CallbackURL,
		&worker.GRPCAddr,
		&worker.Protocol,
		&worker.Status,
		&worker.Labels,
		&worker.MaxConcurrency,
		&worker.CurrentLoad,
		&worker.LastHeartbeatAt,
	); err != nil {
		return nil, fmt.Errorf("scan worker: %w", err)
	}
	return &worker, nil
}

func scanWorkers(rows *sql.Rows) ([]*model.WorkerNode, error) {
	workers := make([]*model.WorkerNode, 0)
	for rows.Next() {
		worker, err := scanWorker(rows)
		if err != nil {
			return nil, err
		}
		workers = append(workers, worker)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workers: %w", err)
	}
	return workers, nil
}
