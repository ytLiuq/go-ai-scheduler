package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/ai/loganalysis"
	"github.com/example/go-ai-scheduler/internal/model"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
	"github.com/example/go-ai-scheduler/internal/repo"
)

// --------------- query_tasks ---------------

type queryTasksTool struct{ bundle *repo.Bundle }

func (t *queryTasksTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "query_tasks",
			Description: "查询任务列表。可按名称模糊匹配、状态筛选。返回任务名称、类型、cron表达式、状态和下次触发时间。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":   map[string]any{"type": "string", "description": "任务名称关键词，支持模糊匹配"},
					"status": map[string]any{"type": "string", "description": "任务状态：enabled 或 disabled"},
					"limit":  map[string]any{"type": "integer", "description": "返回数量上限，默认20"},
				},
			},
		},
	}
}

func (t *queryTasksTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Limit <= 0 {
		p.Limit = 20
	}

	tasks, err := t.bundle.Task.ListTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	filtered := make([]map[string]any, 0)
	for _, task := range tasks {
		if p.Name != "" && !strings.Contains(strings.ToLower(task.Name), strings.ToLower(p.Name)) {
			continue
		}
		if p.Status != "" && task.Status != p.Status {
			continue
		}
		filtered = append(filtered, map[string]any{
			"id":                task.ID,
			"name":              task.Name,
			"type":              task.Type,
			"cron_expr":         task.CronExpr,
			"status":            task.Status,
			"next_trigger_time": task.NextTriggerTime.Format(time.RFC3339),
			"max_retry":         task.MaxRetry,
			"timeout_seconds":   task.TimeoutSeconds,
		})
		if len(filtered) >= p.Limit {
			break
		}
	}
	return map[string]any{"count": len(filtered), "tasks": filtered}, nil
}

// --------------- query_instances ---------------

type queryInstancesTool struct{ bundle *repo.Bundle }

func (t *queryInstancesTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "query_instances",
			Description: "查询任务实例列表。可按状态、任务ID、时间范围筛选。返回实例ID、状态、错误信息、执行时间等。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status":  map[string]any{"type": "string", "description": "实例状态：success, failed, running, timeout, etc."},
					"task_id": map[string]any{"type": "integer", "description": "任务ID"},
					"hours":   map[string]any{"type": "integer", "description": "最近多少小时，默认24"},
					"limit":   map[string]any{"type": "integer", "description": "返回数量上限，默认20"},
				},
			},
		},
	}
}

func (t *queryInstancesTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Status string `json:"status"`
		TaskID int64  `json:"task_id"`
		Hours  int    `json:"hours"`
		Limit  int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Hours <= 0 {
		p.Hours = 24
	}
	if p.Limit <= 0 {
		p.Limit = 20
	}

	instances, err := t.bundle.TaskInstance.ListInstancesByTimeRange(ctx, time.Now().Add(-24*time.Hour), time.Now(), 0, 0)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}

	cutoff := time.Now().Add(-time.Duration(p.Hours) * time.Hour)
	filtered := make([]map[string]any, 0)
	for _, inst := range instances {
		if p.Status != "" && inst.Status != p.Status {
			continue
		}
		if p.TaskID > 0 && inst.TaskID != p.TaskID {
			continue
		}
		if inst.TriggerTime.Before(cutoff) {
			continue
		}
		filtered = append(filtered, map[string]any{
			"id":                   inst.ID,
			"task_id":              inst.TaskID,
			"schedule_instance_id": inst.ScheduleInstanceID,
			"status":               inst.Status,
			"worker_id":            inst.WorkerID,
			"trigger_time":         inst.TriggerTime.Format(time.RFC3339),
			"retry_count":          inst.RetryCount,
			"error_code":           inst.ErrorCode,
			"error_message":        truncateStr(inst.ErrorMessage, 500),
		})
		if len(filtered) >= p.Limit {
			break
		}
	}
	return map[string]any{"count": len(filtered), "instances": filtered}, nil
}

// --------------- query_workers ---------------

type queryWorkersTool struct{ bundle *repo.Bundle }

func (t *queryWorkersTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "query_workers",
			Description: "查询 worker 节点状态。返回每个 worker 的在线状态、当前负载、最大并发、最后心跳时间。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (t *queryWorkersTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	workers, err := t.bundle.Worker.ListWorkers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}

	result := make([]map[string]any, 0)
	online := 0
	for _, w := range workers {
		if w.Status == "online" {
			online++
		}
		utilization := 0.0
		if w.MaxConcurrency > 0 {
			utilization = float64(w.CurrentLoad) / float64(w.MaxConcurrency) * 100
		}
		result = append(result, map[string]any{
			"id":                w.ID,
			"hostname":          w.Hostname,
			"status":            w.Status,
			"current_load":      w.CurrentLoad,
			"max_concurrency":   w.MaxConcurrency,
			"utilization_pct":   fmt.Sprintf("%.1f", utilization),
			"last_heartbeat_at": w.LastHeartbeatAt.Format(time.RFC3339),
		})
	}
	return map[string]any{"total": len(workers), "online": online, "workers": result}, nil
}

// --------------- get_task_detail ---------------

type getTaskDetailTool struct{ bundle *repo.Bundle }

func (t *getTaskDetailTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "get_task_detail",
			Description: "获取某个任务的详细信息，包括依赖关系、最近执行实例。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "integer", "description": "任务ID"},
				},
				"required": []string{"task_id"},
			},
		},
	}
}

func (t *getTaskDetailTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		TaskID int64 `json:"task_id"`
	}
	_ = json.Unmarshal(args, &p)
	if p.TaskID <= 0 {
		return nil, fmt.Errorf("task_id is required")
	}

	task, err := t.bundle.Task.GetTask(ctx, p.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task %d not found", p.TaskID)
	}

	upstream, _ := t.bundle.Task.ListUpstreamDeps(ctx, p.TaskID)
	downstream, _ := t.bundle.Task.ListDownstreamTasks(ctx, p.TaskID)

	// Get recent instances.
	instances, _ := t.bundle.TaskInstance.ListInstancesByTimeRange(ctx, time.Now().Add(-24*time.Hour), time.Now(), 0, 0)
	recentInstances := make([]map[string]any, 0)
	count := 0
	for _, inst := range instances {
		if inst.TaskID == p.TaskID && count < 10 {
			recentInstances = append(recentInstances, map[string]any{
				"id":        inst.ID,
				"status":    inst.Status,
				"trigger_time": inst.TriggerTime.Format(time.RFC3339),
				"retry_count":  inst.RetryCount,
				"error_code":   inst.ErrorCode,
			})
			count++
		}
	}

	return map[string]any{
		"id":                task.ID,
		"name":              task.Name,
		"type":              task.Type,
		"cron_expr":         task.CronExpr,
		"payload":           truncateStr(task.Payload, 500),
		"status":            task.Status,
		"timeout_seconds":   task.TimeoutSeconds,
		"max_retry":         task.MaxRetry,
		"retry_policy":      task.RetryPolicy,
		"route_strategy":    task.RouteStrategy,
		"next_trigger_time": task.NextTriggerTime.Format(time.RFC3339),
		"upstream_deps":     upstream,
		"downstream_deps":   downstream,
		"recent_instances":  recentInstances,
	}, nil
}

// --------------- get_system_health ---------------

type getSystemHealthTool struct{ bundle *repo.Bundle }

func (t *getSystemHealthTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "get_system_health",
			Description: "获取系统整体健康状态概览：任务总数、启用任务数、worker在线率、最近失败实例数、各状态实例统计。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (t *getSystemHealthTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	tasks, _ := t.bundle.Task.ListTasks(ctx)
	workers, _ := t.bundle.Worker.ListWorkers(ctx)
	instances, _ := t.bundle.TaskInstance.ListInstancesByTimeRange(ctx, time.Now().Add(-24*time.Hour), time.Now(), 0, 0)

	totalTasks := len(tasks)
	enabledTasks := 0
	disabledTasks := 0
	for _, t := range tasks {
		switch t.Status {
		case "enabled":
			enabledTasks++
		default:
			disabledTasks++
		}
	}

	totalWorkers := len(workers)
	onlineWorkers := 0
	totalLoad := 0
	totalCapacity := 0
	for _, w := range workers {
		if w.Status == "online" {
			onlineWorkers++
		}
		totalLoad += w.CurrentLoad
		totalCapacity += w.MaxConcurrency
	}
	avgLoad := 0.0
	if totalCapacity > 0 {
		avgLoad = float64(totalLoad) / float64(totalCapacity) * 100
	}

	// Instance stats for last 24h (query already filters to last 24h).
	recentTotal := 0
	recentFailed := 0
	recentSuccess := 0
	recentRunning := 0
	for _, inst := range instances {
		recentTotal++
		switch inst.Status {
		case "success":
			recentSuccess++
		case "failed", "timeout":
			recentFailed++
		case "running":
			recentRunning++
		}
	}

	failureRate := 0.0
	if recentTotal > 0 {
		failureRate = float64(recentFailed) / float64(recentTotal) * 100
	}

	return map[string]any{
		"tasks": map[string]any{
			"total":    totalTasks,
			"enabled":  enabledTasks,
			"disabled": disabledTasks,
		},
		"workers": map[string]any{
			"total":          totalWorkers,
			"online":         onlineWorkers,
			"avg_load_pct":   fmt.Sprintf("%.1f", avgLoad),
		},
		"instances_24h": map[string]any{
			"total":        recentTotal,
			"success":      recentSuccess,
			"failed":       recentFailed,
			"running":      recentRunning,
			"failure_rate": fmt.Sprintf("%.1f%%", failureRate),
		},
	}, nil
}

// --------------- analyze_failure ---------------

type analyzeFailureTool struct{ bundle *repo.Bundle }

func (t *analyzeFailureTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "analyze_failure",
			Description: "分析一个失败任务实例的根因。输入实例ID，返回严重程度、根因分析、修复建议。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id": map[string]any{"type": "integer", "description": "失败实例的ID"},
				},
				"required": []string{"instance_id"},
			},
		},
	}
}

func (t *analyzeFailureTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		InstanceID int64 `json:"instance_id"`
	}
	_ = json.Unmarshal(args, &p)
	if p.InstanceID <= 0 {
		return nil, fmt.Errorf("instance_id is required")
	}

	inst, err := t.bundle.TaskInstance.GetInstance(ctx, p.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}

	task, err := t.bundle.Task.GetTask(ctx, inst.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	taskType := ""
	if task != nil {
		taskType = task.Type
	}

	// Reuse existing loganalysis (which requires an LLM adapter - passed via context or directly).
	// For the initial implementation, return the raw error info.
	// The LLM in the agent loop will provide the actual analysis.
	return map[string]any{
		"instance_id":    inst.ID,
		"task_id":        inst.TaskID,
		"task_type":      taskType,
		"status":         inst.Status,
		"error_code":     inst.ErrorCode,
		"error_message":  inst.ErrorMessage,
		"retry_count":    inst.RetryCount,
		"trigger_time":   inst.TriggerTime.Format(time.RFC3339),
		"worker_id":      inst.WorkerID,
	}, nil
}

// --------------- create_task ---------------

type createTaskTool struct{ bundle *repo.Bundle }

func (t *createTaskTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "create_task",
			Description: "创建一个新任务。需要提供任务名称、类型、cron表达式、执行内容和可选参数。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":            map[string]any{"type": "string", "description": "任务名称，必须唯一"},
					"type":            map[string]any{"type": "string", "enum": []string{"shell", "http", "container"}, "description": "任务类型"},
					"cron_expr":       map[string]any{"type": "string", "description": "cron 表达式，如 */5 * * * *"},
					"payload":         map[string]any{"type": "string", "description": "执行内容：shell命令、HTTP URL 或容器参数"},
					"image":           map[string]any{"type": "string", "description": "容器镜像（仅 container 类型需要）"},
					"timeout_seconds": map[string]any{"type": "integer", "description": "超时秒数，默认300"},
					"max_retry":       map[string]any{"type": "integer", "description": "最大重试次数，默认3"},
					"retry_policy":    map[string]any{"type": "string", "enum": []string{"fixed_interval", "exponential_backoff"}, "description": "重试策略"},
				},
				"required": []string{"name", "type", "cron_expr", "payload"},
			},
		},
	}
}

func (t *createTaskTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Name           string `json:"name"`
		Type           string `json:"type"`
		CronExpr       string `json:"cron_expr"`
		Payload        string `json:"payload"`
		Image          string `json:"image"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		MaxRetry       int    `json:"max_retry"`
		RetryPolicy    string `json:"retry_policy"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if p.Name == "" || p.Type == "" || p.CronExpr == "" || p.Payload == "" {
		return nil, fmt.Errorf("name, type, cron_expr, and payload are required")
	}
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = 300
	}
	if p.MaxRetry <= 0 {
		p.MaxRetry = 3
	}
	if p.RetryPolicy == "" {
		p.RetryPolicy = "fixed_interval"
	}

	// Validate cron.
	if _, err := cronexpr.NextAfter(time.Now(), p.CronExpr); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %s", p.CronExpr)
	}

	nextTrigger, _ := cronexpr.NextAfter(time.Now(), p.CronExpr)

	task := &model.Task{
		Name:            p.Name,
		Type:            p.Type,
		CronExpr:        p.CronExpr,
		Payload:         p.Payload,
		Image:           p.Image,
		Status:          "enabled",
		TimeoutSeconds:  p.TimeoutSeconds,
		MaxRetry:        p.MaxRetry,
		RetryPolicy:     p.RetryPolicy,
		RouteStrategy:   "round_robin",
		NextTriggerTime: nextTrigger,
	}
	if err := t.bundle.Task.CreateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return map[string]any{
		"created": true,
		"task_id": task.ID,
		"name":    task.Name,
		"message": fmt.Sprintf("任务 %s (ID: %d) 创建成功，下次触发时间 %s", task.Name, task.ID, nextTrigger.Format(time.RFC3339)),
	}, nil
}

// --------------- trigger_task ---------------

type triggerTaskTool struct{ bundle *repo.Bundle }

func (t *triggerTaskTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "trigger_task",
			Description: "手动触发一个任务立即执行。将任务的下次触发时间设为当前时间。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "integer", "description": "要触发的任务ID"},
				},
				"required": []string{"task_id"},
			},
		},
	}
}

func (t *triggerTaskTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		TaskID int64 `json:"task_id"`
	}
	_ = json.Unmarshal(args, &p)
	if p.TaskID <= 0 {
		return nil, fmt.Errorf("task_id is required")
	}

	task, err := t.bundle.Task.GetTask(ctx, p.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task %d not found", p.TaskID)
	}

	task.NextTriggerTime = time.Now()
	if err := t.bundle.Task.UpdateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}
	return map[string]any{
		"triggered": true,
		"task_id":   task.ID,
		"name":      task.Name,
		"message":   fmt.Sprintf("任务 %s 已触发，将在下一个调度周期执行", task.Name),
	}, nil
}

// --------------- pause_task ---------------

type pauseTaskTool struct{ bundle *repo.Bundle }

func (t *pauseTaskTool) Definition() adapter.Tool {
	return adapter.Tool{
		Type: "function",
		Function: adapter.FunctionDef{
			Name:        "pause_task",
			Description: "暂停或恢复一个任务。设置 action 为 pause 暂停，resume 恢复。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "integer", "description": "任务ID"},
					"action":  map[string]any{"type": "string", "enum": []string{"pause", "resume"}, "description": "pause 暂停或 resume 恢复"},
				},
				"required": []string{"task_id", "action"},
			},
		},
	}
}

func (t *pauseTaskTool) Execute(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		TaskID int64  `json:"task_id"`
		Action string `json:"action"`
	}
	_ = json.Unmarshal(args, &p)
	if p.TaskID <= 0 {
		return nil, fmt.Errorf("task_id is required")
	}

	task, err := t.bundle.Task.GetTask(ctx, p.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task %d not found", p.TaskID)
	}

	switch p.Action {
	case "pause":
		if task.Status == "disabled" {
			return map[string]any{"message": "任务已经是暂停状态"}, nil
		}
		task.Status = "disabled"
	case "resume":
		if task.Status == "enabled" {
			return map[string]any{"message": "任务已经是运行状态"}, nil
		}
		task.Status = "enabled"
	default:
		return nil, fmt.Errorf("action must be pause or resume, got %q", p.Action)
	}

	if err := t.bundle.Task.UpdateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}
	return map[string]any{
		"task_id": task.ID,
		"name":    task.Name,
		"status":  task.Status,
		"message": fmt.Sprintf("任务 %s 已%s", task.Name, map[string]string{"pause": "暂停", "resume": "恢复"}[p.Action]),
	}, nil
}

// --------------- helpers ---------------

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// AnalyzeInstance analyzes a failed instance using LLM and returns the analysis.
// This is called externally by the agent when it has an LLM adapter available.
func AnalyzeInstance(llm *adapter.LLMAdapter, inst *model.TaskInstance, taskType string) (*loganalysis.AnalysisResponse, error) {
	if llm == nil {
		return nil, fmt.Errorf("LLM not configured")
	}
	return loganalysis.AnalyzeWithLLM(context.Background(), llm, inst.ErrorMessage, inst.ErrorCode, taskType, inst.RetryCount)
}
