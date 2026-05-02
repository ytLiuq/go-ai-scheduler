package advisor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/ai/prompts"
)

// ErrLLMRequired is returned when an LLM is not available and the operation
// cannot fall back to a heuristic implementation.
var ErrLLMRequired = fmt.Errorf("llm adapter not configured or disabled")

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
	AvgWorkerLoad       float64 `json:"avg_worker_load"`
	TotalWorkers        int     `json:"total_workers"`
	OnlineWorkers       int     `json:"online_workers"`
	PendingInstances    int     `json:"pending_instances"`
	FailedLastHour      int     `json:"failed_last_hour"`
	AvgDispatchLatencyMs float64 `json:"avg_dispatch_latency_ms"`
	MaxPendingConfig    int     `json:"max_pending_config"`
}

// Generate produces scheduling recommendations via LLM. Returns ErrLLMRequired
// if no LLM adapter is configured.
func Generate(ctx context.Context, llm *adapter.LLMAdapter, sctx Context) ([]Advice, error) {
	if llm == nil || !llm.Enabled() {
		return nil, ErrLLMRequired
	}

	systemPrompt := prompts.Advisor

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
		return nil, fmt.Errorf("llm advisor: %w", err)
	}
	var advices []Advice
	if err := json.Unmarshal([]byte(result), &advices); err != nil {
		return nil, fmt.Errorf("parse llm advisor output: %w (raw=%s)", err, result)
	}
	return advices, nil
}
