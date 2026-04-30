package rpc

// ExecuteTaskRequest is the shared task dispatch payload, used by both HTTP and gRPC paths.
type ExecuteTaskRequest struct {
	ScheduleInstanceID string `json:"schedule_instance_id"`
	TaskID             int64  `json:"task_id"`
	TaskType           string `json:"task_type"`
	Payload            string `json:"payload"`
	Image              string `json:"image"`
	TimeoutSeconds     int    `json:"timeout_seconds"`
	RetryCount         int    `json:"retry_count"`
	ShardNo            int    `json:"shard_no"`
	ShardTotal         int    `json:"shard_total"`
	IdempotencyKey     string `json:"idempotency_key"`
	SchedulerURL       string `json:"scheduler_url"`
}
