CREATE TABLE IF NOT EXISTS task (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(128) NOT NULL,
    type VARCHAR(32) NOT NULL,
    cron_expr VARCHAR(64) NOT NULL DEFAULT '',
    payload TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'enabled',
    timeout_seconds INT NOT NULL DEFAULT 300,
    max_retry INT NOT NULL DEFAULT 3,
    retry_policy VARCHAR(32) NOT NULL DEFAULT 'fixed_interval',
    route_strategy VARCHAR(32) NOT NULL DEFAULT 'round_robin',
    next_trigger_time DATETIME NULL,
    tenant_id BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_task_name_tenant (tenant_id, name),
    KEY idx_task_status_next_trigger (status, next_trigger_time)
);

CREATE TABLE IF NOT EXISTS task_instance (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id BIGINT NOT NULL,
    schedule_instance_id VARCHAR(64) NOT NULL,
    trigger_time DATETIME NOT NULL,
    dispatch_time DATETIME NULL,
    worker_id VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL,
    trace_id VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_schedule_instance_id (schedule_instance_id),
    KEY idx_task_instance_task_id (task_id),
    KEY idx_task_instance_status_trigger (status, trigger_time)
);

CREATE TABLE IF NOT EXISTS worker_node (
    id VARCHAR(64) PRIMARY KEY,
    hostname VARCHAR(128) NOT NULL,
    ip VARCHAR(64) NOT NULL,
    callback_url VARCHAR(255) NOT NULL DEFAULT '',
    grpc_addr VARCHAR(128) NOT NULL DEFAULT '',
    protocol VARCHAR(16) NOT NULL DEFAULT 'http',
    status VARCHAR(32) NOT NULL DEFAULT 'online',
    labels TEXT NOT NULL,
    max_concurrency INT NOT NULL DEFAULT 100,
    current_load INT NOT NULL DEFAULT 0,
    last_heartbeat_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_worker_status_heartbeat (status, last_heartbeat_at)
);

CREATE TABLE IF NOT EXISTS ai_analysis_record (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    instance_id BIGINT NOT NULL,
    analysis_type VARCHAR(32) NOT NULL,
    input_snapshot JSON NOT NULL,
    output_json JSON NOT NULL,
    confidence DECIMAL(5,4) NOT NULL DEFAULT 0.0000,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_ai_analysis_instance_type (instance_id, analysis_type)
);
