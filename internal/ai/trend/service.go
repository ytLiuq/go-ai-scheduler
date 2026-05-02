package trend

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/model"
)

var ErrLLMRequired = fmt.Errorf("llm adapter not configured or disabled")

// TrendItem describes one observed trend.
type TrendItem struct {
	Metric    string `json:"metric"`
	Direction string `json:"direction"`
	Detail    string `json:"detail"`
}

// Recommendation is an actionable suggestion.
type Recommendation struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Urgency     string `json:"urgency"`
}

// DataSnapshot contains the computed system metrics sent to the LLM.
type DataSnapshot struct {
	WorkerCount      int     `json:"worker_count"`
	OnlineWorkers    int     `json:"online_workers"`
	AvgWorkerLoad    float64 `json:"avg_worker_load"`
	TotalInstances   int     `json:"total_instances"`
	FailedInstances  int     `json:"failed_instances"`
	SuccessInstances int     `json:"success_instances"`
	PendingInstances int     `json:"pending_instances"`
	TaskCount        int     `json:"task_count"`
	EnabledTasks     int     `json:"enabled_tasks"`
	TimeWindowHours  int     `json:"time_window_hours"`
}

// TrendResponse is the full LLM response.
type TrendResponse struct {
	OverallAssessment string           `json:"overall_assessment"`
	Trends            []TrendItem      `json:"trends"`
	Recommendations   []Recommendation `json:"recommendations"`
	DataSnapshot      DataSnapshot     `json:"data_snapshot"`
}

// ComputeSnapshot gathers system metrics from all repositories.
func ComputeSnapshot(workers []*model.WorkerNode, tasks []*model.Task, instances []*model.TaskInstance, timeWindowHours int) DataSnapshot {
	snapshot := DataSnapshot{
		WorkerCount: len(workers),
		TaskCount:   len(tasks),
	}
	var onlineCount int
	var totalLoad float64
	for _, w := range workers {
		if w.Status == "online" {
			onlineCount++
			if w.MaxConcurrency > 0 {
				totalLoad += float64(w.CurrentLoad) / float64(w.MaxConcurrency)
			}
		}
	}
	snapshot.OnlineWorkers = onlineCount
	if onlineCount > 0 {
		snapshot.AvgWorkerLoad = totalLoad / float64(onlineCount)
	}

	for _, t := range tasks {
		if t.Status == "enabled" {
			snapshot.EnabledTasks++
		}
	}

	windowCutoff := time.Now().Add(-time.Duration(timeWindowHours) * time.Hour)
	for _, inst := range instances {
		if timeWindowHours > 0 && inst.CreatedAt.Before(windowCutoff) {
			continue
		}
		snapshot.TotalInstances++
		switch inst.Status {
		case "failed":
			snapshot.FailedInstances++
		case "success":
			snapshot.SuccessInstances++
		case "pending", "dispatched", "running", "retry_waiting":
			snapshot.PendingInstances++
		}
	}
	snapshot.TimeWindowHours = timeWindowHours
	return snapshot
}

// AnalyzeWithLLM sends the data snapshot to the LLM for trend analysis.
func AnalyzeWithLLM(ctx context.Context, llm *adapter.LLMAdapter, snapshot DataSnapshot) (*TrendResponse, error) {
	if llm == nil || !llm.Enabled() {
		return nil, ErrLLMRequired
	}

	systemPrompt := `You are a system reliability analyst reviewing scheduler metrics. Analyze the current system state and identify trends. Return ONLY valid JSON:
{"overall_assessment": "<one-paragraph assessment>", "trends": [{"metric": "<metric_name>", "direction": "increasing|decreasing|stable", "detail": "<detail>"}], "recommendations": [{"type": "scale|throttle|migrate|config|investigate", "title": "<short title>", "description": "<detail>", "urgency": "low|medium|high"}]}`

	userPrompt := fmt.Sprintf(`Time window: %d hours

Worker stats:
- Total workers: %d (online: %d)
- Average worker load: %.2f%%

Task stats:
- Total tasks: %d (enabled: %d)

Instance stats (within window):
- Total instances: %d
- Failed: %d
- Success: %d
- Pending/in-flight: %d`,
		snapshot.TimeWindowHours,
		snapshot.WorkerCount, snapshot.OnlineWorkers, snapshot.AvgWorkerLoad*100,
		snapshot.TaskCount, snapshot.EnabledTasks,
		snapshot.TotalInstances, snapshot.FailedInstances, snapshot.SuccessInstances, snapshot.PendingInstances)

	result, err := llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("llm trend analysis: %w", err)
	}

	var resp TrendResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("parse llm trend output: %w (raw=%s)", err, result)
	}
	resp.DataSnapshot = snapshot
	return &resp, nil
}
