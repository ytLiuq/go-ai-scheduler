package advisor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
)

// Advice is a structured recommendation.
type Advice struct {
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
	AutoApply   bool    `json:"auto_apply"`
}

// Context provides the input for schedule advice generation.
type Context struct {
	AvgWorkerLoad     float64 `json:"avg_worker_load"`
	TotalWorkers      int     `json:"total_workers"`
	OnlineWorkers     int     `json:"online_workers"`
	PendingInstances  int     `json:"pending_instances"`
	FailedLastHour    int     `json:"failed_last_hour"`
	AvgDispatchLatencyMs float64 `json:"avg_dispatch_latency_ms"`
	MaxPendingConfig  int     `json:"max_pending_config"`
}

// Generate produces scheduling recommendations. Falls back to heuristic if no LLM.
func Generate(ctx context.Context, llm *adapter.LLMAdapter, sctx Context) ([]Advice, error) {
	if llm != nil && llm.Enabled() {
		if advices := llmGenerate(ctx, llm, sctx); len(advices) > 0 {
			return advices, nil
		}
	}
	return heuristicGenerate(sctx), nil
}

func llmGenerate(ctx context.Context, llm *adapter.LLMAdapter, sctx Context) []Advice {
	systemPrompt := `You are a workload scheduling advisor. Given the scheduler metrics below, suggest actionable recommendations. Return ONLY valid JSON array:
[{"type": "<throttle|migrate|scale|config>", "title": "<title>", "description": "<detail>", "confidence": <0.0-1.0>, "auto_apply": false}]`

	userPrompt := fmt.Sprintf(`Avg worker load: %.2f%%
Total workers: %d (online: %d)
Pending instances: %d (max: %d)
Failed in last hour: %d
Avg dispatch latency: %.2f ms`,
		sctx.AvgWorkerLoad*100, sctx.TotalWorkers, sctx.OnlineWorkers,
		sctx.PendingInstances, sctx.MaxPendingConfig,
		sctx.FailedLastHour, sctx.AvgDispatchLatencyMs)

	result, err := llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil
	}
	var advices []Advice
	if err := json.Unmarshal([]byte(result), &advices); err != nil {
		return nil
	}
	return advices
}

func heuristicGenerate(sctx Context) []Advice {
	var advices []Advice

	if sctx.OnlineWorkers < sctx.TotalWorkers {
		advices = append(advices, Advice{
			Type:        "scale",
			Title:       "Workers offline",
			Description: fmt.Sprintf("%d of %d workers are offline. Check worker health.", sctx.TotalWorkers-sctx.OnlineWorkers, sctx.TotalWorkers),
			Confidence:  0.9,
		})
	}

	if sctx.AvgWorkerLoad > 0.8 {
		advices = append(advices, Advice{
			Type:        "scale",
			Title:       "High worker load",
			Description: fmt.Sprintf("Average worker load is %.0f%%. Consider adding more workers.", sctx.AvgWorkerLoad*100),
			Confidence:  0.85,
		})
	}

	if sctx.PendingInstances > int(float64(sctx.MaxPendingConfig)*0.8) {
		advices = append(advices, Advice{
			Type:        "throttle",
			Title:       "Backpressure building",
			Description: fmt.Sprintf("%d pending instances (limit %d). Increase max_pending or add workers.", sctx.PendingInstances, sctx.MaxPendingConfig),
			Confidence:  0.8,
		})
	}

	if sctx.FailedLastHour > 10 {
		advices = append(advices, Advice{
			Type:        "config",
			Title:       "Elevated failure rate",
			Description: fmt.Sprintf("%d failures in the last hour. Review error patterns and consider adjusting retry policy.", sctx.FailedLastHour),
			Confidence:  0.75,
		})
	}

	if sctx.AvgDispatchLatencyMs > 100 {
		advices = append(advices, Advice{
			Type:        "config",
			Title:       "High dispatch latency",
			Description: fmt.Sprintf("Avg dispatch latency is %.0f ms. Check network or scheduler throughput.", sctx.AvgDispatchLatencyMs),
			Confidence:  0.7,
		})
	}

	if len(advices) == 0 {
		advices = append(advices, Advice{
			Type:        "info",
			Title:       "System healthy",
			Description: "No issues detected. All metrics within normal range.",
			Confidence:  0.9,
		})
	}
	return advices
}
