ALTER TABLE task_instance ADD COLUMN started_at DATETIME NULL AFTER dispatch_time;
ALTER TABLE task_instance ADD COLUMN finished_at DATETIME NULL AFTER started_at;
