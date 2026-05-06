package mysql

import "time"

type taskRow struct {
	ID              int64      `gorm:"column:id;primaryKey"`
	Name            string     `gorm:"column:name"`
	Type            string     `gorm:"column:type"`
	CronExpr        string     `gorm:"column:cron_expr"`
	Payload         string     `gorm:"column:payload"`
	Image           string     `gorm:"column:image"`
	Status          string     `gorm:"column:status"`
	TimeoutSeconds  int        `gorm:"column:timeout_seconds"`
	MaxRetry        int        `gorm:"column:max_retry"`
	RetryPolicy     string     `gorm:"column:retry_policy"`
	RetryOnErrors   string     `gorm:"column:retry_on_errors"`
	RouteStrategy   string     `gorm:"column:route_strategy"`
	Labels          string     `gorm:"column:labels"`
	NextTriggerTime *time.Time `gorm:"column:next_trigger_time"`
	TenantID        int64      `gorm:"column:tenant_id"`
	Version         int64      `gorm:"column:version"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (taskRow) TableName() string { return "task" }

type taskDependencyRow struct {
	TaskID          int64 `gorm:"column:task_id"`
	DependsOnTaskID int64 `gorm:"column:depends_on_task_id"`
}

func (taskDependencyRow) TableName() string { return "task_dependency" }

type taskInstanceRow struct {
	ID                 int64      `gorm:"column:id;primaryKey"`
	TaskID             int64      `gorm:"column:task_id"`
	ScheduleInstanceID string     `gorm:"column:schedule_instance_id"`
	TriggerTime        time.Time  `gorm:"column:trigger_time"`
	DispatchTime       *time.Time `gorm:"column:dispatch_time"`
	StartedAt          *time.Time `gorm:"column:started_at"`
	FinishedAt         *time.Time `gorm:"column:finished_at"`
	WorkerID           string     `gorm:"column:worker_id"`
	Status             string     `gorm:"column:status"`
	RetryCount         int        `gorm:"column:retry_count"`
	ErrorCode          string     `gorm:"column:error_code"`
	ErrorMessage       string     `gorm:"column:error_message"`
	AnalysisJSON       string     `gorm:"column:analysis_json"`
	TraceID            string     `gorm:"column:trace_id"`
	NextRetryTime      *time.Time `gorm:"column:next_retry_time"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (taskInstanceRow) TableName() string { return "task_instance" }

type workerNodeRow struct {
	ID              string    `gorm:"column:id;primaryKey"`
	Hostname        string    `gorm:"column:hostname"`
	IP              string    `gorm:"column:ip"`
	CallbackURL     string    `gorm:"column:callback_url"`
	GRPCAddr        string    `gorm:"column:grpc_addr"`
	Protocol        string    `gorm:"column:protocol"`
	Status          string    `gorm:"column:status"`
	Labels          string    `gorm:"column:labels"`
	MaxConcurrency  int       `gorm:"column:max_concurrency"`
	CurrentLoad     int       `gorm:"column:current_load"`
	LastHeartbeatAt time.Time `gorm:"column:last_heartbeat_at"`
}

func (workerNodeRow) TableName() string { return "worker_node" }

type aiAnalysisRecordRow struct {
	ID           int64     `gorm:"column:id;primaryKey"`
	InstanceID   int64     `gorm:"column:instance_id"`
	AnalysisType string    `gorm:"column:analysis_type"`
	InputJSON    string    `gorm:"column:input_snapshot"`
	OutputJSON   string    `gorm:"column:output_json"`
	Confidence   float64   `gorm:"column:confidence"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (aiAnalysisRecordRow) TableName() string { return "ai_analysis_record" }

type workerLoadSnapshotRow struct {
	ID             int64     `gorm:"column:id;primaryKey"`
	WorkerID       string    `gorm:"column:worker_id"`
	CurrentLoad    int       `gorm:"column:current_load"`
	MaxConcurrency int       `gorm:"column:max_concurrency"`
	Status         string    `gorm:"column:status"`
	RecordedAt     time.Time `gorm:"column:recorded_at"`
}

func (workerLoadSnapshotRow) TableName() string { return "worker_load_snapshot" }
