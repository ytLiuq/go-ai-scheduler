package rpc

// ExecuteTaskRequest is the shared task dispatch payload, used by both HTTP and gRPC paths.
type ExecuteTaskRequest struct {
	ScheduleInstanceID string `json:"schedule_instance_id"`
	TaskID             int64  `json:"task_id"`
	TaskType           string `json:"task_type"`
	Payload            string `json:"payload"`
	TimeoutSeconds     int    `json:"timeout_seconds"`
	RetryCount         int    `json:"retry_count"`
	SchedulerURL       string `json:"scheduler_url"`
}
