CREATE TABLE IF NOT EXISTS worker_load_snapshot (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    worker_id VARCHAR(64) NOT NULL,
    current_load INT NOT NULL DEFAULT 0,
    max_concurrency INT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'online',
    recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_snapshot_worker_time (worker_id, recorded_at),
    KEY idx_snapshot_time (recorded_at)
);
