-- +migrate Up
CREATE TABLE IF NOT EXISTS task_dependency (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    depends_on_task_id BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_task_depends (task_id, depends_on_task_id),
    FOREIGN KEY (task_id) REFERENCES task(id) ON DELETE CASCADE,
    FOREIGN KEY (depends_on_task_id) REFERENCES task(id) ON DELETE CASCADE
);

-- +migrate Down
DROP TABLE IF EXISTS task_dependency;
