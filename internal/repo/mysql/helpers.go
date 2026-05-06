package mysql

import (
	"time"

	"github.com/example/go-ai-scheduler/internal/model"
)

func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func taskToRow(task *model.Task) taskRow {
	return taskRow{
		ID:              task.ID,
		Name:            task.Name,
		Type:            task.Type,
		CronExpr:        task.CronExpr,
		Payload:         task.Payload,
		Image:           task.Image,
		Status:          task.Status,
		TimeoutSeconds:  task.TimeoutSeconds,
		MaxRetry:        task.MaxRetry,
		RetryPolicy:     task.RetryPolicy,
		RetryOnErrors:   task.RetryOnErrors,
		RouteStrategy:   task.RouteStrategy,
		Labels:          task.Labels,
		NextTriggerTime: timeOrNil(task.NextTriggerTime),
		TenantID:        task.TenantID,
		Version:         task.Version,
		CreatedAt:       task.CreatedAt,
		UpdatedAt:       task.UpdatedAt,
	}
}

func rowToTask(row *taskRow) *model.Task {
	task := &model.Task{
		ID:             row.ID,
		Name:           row.Name,
		Type:           row.Type,
		CronExpr:       row.CronExpr,
		Payload:        row.Payload,
		Image:          row.Image,
		Status:         row.Status,
		TimeoutSeconds: row.TimeoutSeconds,
		MaxRetry:       row.MaxRetry,
		RetryPolicy:    row.RetryPolicy,
		RetryOnErrors:  row.RetryOnErrors,
		RouteStrategy:  row.RouteStrategy,
		Labels:         row.Labels,
		TenantID:       row.TenantID,
		Version:        row.Version,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.NextTriggerTime != nil {
		task.NextTriggerTime = *row.NextTriggerTime
	}
	return task
}

func taskInstanceToRow(instance *model.TaskInstance) taskInstanceRow {
	return taskInstanceRow{
		ID:                 instance.ID,
		TaskID:             instance.TaskID,
		ScheduleInstanceID: instance.ScheduleInstanceID,
		TriggerTime:        instance.TriggerTime,
		DispatchTime:       timeOrNil(instance.DispatchTime),
		StartedAt:          timeOrNil(instance.StartedAt),
		FinishedAt:         timeOrNil(instance.FinishedAt),
		WorkerID:           instance.WorkerID,
		Status:             instance.Status,
		RetryCount:         instance.RetryCount,
		ErrorCode:          instance.ErrorCode,
		ErrorMessage:       instance.ErrorMessage,
		AnalysisJSON:       instance.AnalysisJSON,
		TraceID:            instance.TraceID,
		NextRetryTime:      timeOrNil(instance.NextRetryTime),
		CreatedAt:          instance.CreatedAt,
		UpdatedAt:          instance.UpdatedAt,
	}
}

func rowToTaskInstance(row *taskInstanceRow) *model.TaskInstance {
	instance := &model.TaskInstance{
		ID:                 row.ID,
		TaskID:             row.TaskID,
		ScheduleInstanceID: row.ScheduleInstanceID,
		TriggerTime:        row.TriggerTime,
		WorkerID:           row.WorkerID,
		Status:             row.Status,
		RetryCount:         row.RetryCount,
		ErrorCode:          row.ErrorCode,
		ErrorMessage:       row.ErrorMessage,
		AnalysisJSON:       row.AnalysisJSON,
		TraceID:            row.TraceID,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	if row.DispatchTime != nil {
		instance.DispatchTime = *row.DispatchTime
	}
	if row.StartedAt != nil {
		instance.StartedAt = *row.StartedAt
	}
	if row.FinishedAt != nil {
		instance.FinishedAt = *row.FinishedAt
	}
	if row.NextRetryTime != nil {
		instance.NextRetryTime = *row.NextRetryTime
	}
	return instance
}

func rowToWorker(row *workerNodeRow) *model.WorkerNode {
	return &model.WorkerNode{
		ID:              row.ID,
		Hostname:        row.Hostname,
		IP:              row.IP,
		CallbackURL:     row.CallbackURL,
		GRPCAddr:        row.GRPCAddr,
		Protocol:        row.Protocol,
		Status:          row.Status,
		Labels:          row.Labels,
		MaxConcurrency:  row.MaxConcurrency,
		CurrentLoad:     row.CurrentLoad,
		LastHeartbeatAt: row.LastHeartbeatAt,
	}
}
