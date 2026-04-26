package rpc

// Scheduler gRPC methods.
const (
	RegisterWorkerMethod   = "/scheduler.v1.WorkerControlService/RegisterWorker"
	HeartbeatMethod        = "/scheduler.v1.WorkerControlService/Heartbeat"
	ReportTaskStatusMethod = "/scheduler.v1.WorkerControlService/ReportTaskStatus"
)

// Worker gRPC methods.
const (
	ExecuteTaskMethod = "/worker.v1.ExecutorService/ExecuteTask"
)

// ExecuteTaskRequest is the payload sent from scheduler to worker.
type ExecuteTaskRequest struct {
	ScheduleInstanceID string `json:"schedule_instance_id"`
	TaskID             int64  `json:"task_id"`
	TaskType           string `json:"task_type"`
	Payload            string `json:"payload"`
	TimeoutSeconds     int    `json:"timeout_seconds"`
	RetryCount         int    `json:"retry_count"`
	SchedulerURL       string `json:"scheduler_url"`
}

