package predictduration

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/model"
)

var ErrLLMRequired = fmt.Errorf("llm adapter not configured or disabled")

// DurationStats contains aggregated historical execution durations.
type DurationStats struct {
	Count       int     `json:"count"`
	AvgDuration float64 `json:"avg_duration"`
	MinDuration float64 `json:"min_duration"`
	MaxDuration float64 `json:"max_duration"`
	P50Duration float64 `json:"p50_duration"`
	P95Duration float64 `json:"p95_duration"`
}

// PredictResponse is the LLM output for duration prediction.
type PredictResponse struct {
	PredictedDurationSeconds float64       `json:"predicted_duration_seconds"`
	Confidence               float64       `json:"confidence"`
	Trend                    string        `json:"trend"`
	Explanation              string        `json:"explanation"`
	HistoricalStats          DurationStats `json:"historical_stats"`
}

// ComputeStats computes duration statistics from completed instances.
// Duration = updated_at - dispatch_time for instances with status "success" or "failed".
func ComputeStats(instances []*model.TaskInstance) DurationStats {
	var durations []float64
	for _, inst := range instances {
		if inst.Status != "success" && inst.Status != "failed" {
			continue
		}
		if inst.DispatchTime.IsZero() {
			continue
		}
		dur := inst.UpdatedAt.Sub(inst.DispatchTime).Seconds()
		if dur < 0 {
			dur = 0
		}
		durations = append(durations, dur)
	}
	stats := DurationStats{Count: len(durations)}
	if len(durations) == 0 {
		return stats
	}
	sort.Float64s(durations)
	stats.MinDuration = durations[0]
	stats.MaxDuration = durations[len(durations)-1]
	var sum float64
	for _, d := range durations {
		sum += d
	}
	stats.AvgDuration = sum / float64(len(durations))
	stats.P50Duration = percentile(durations, 50)
	stats.P95Duration = percentile(durations, 95)
	return stats
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100.0*float64(len(sorted))) - 1)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// PredictWithLLM sends task config + historical stats to the LLM for duration prediction.
func PredictWithLLM(ctx context.Context, llm *adapter.LLMAdapter, task *model.Task, stats DurationStats) (*PredictResponse, error) {
	if llm == nil || !llm.Enabled() {
		return nil, ErrLLMRequired
	}

	systemPrompt := `You are a task execution time prediction expert. Given historical execution data and task configuration, predict the expected duration of the next execution. Return ONLY valid JSON:
{"predicted_duration_seconds": <float>, "confidence": <0.0-1.0>, "trend": "<stable|increasing|decreasing|volatile>", "explanation": "<one-line explanation>"}`

	userPrompt := fmt.Sprintf(`Task configuration:
- Name: %s
- Type: %s
- Timeout: %d seconds
- Max retries: %d
- Retry policy: %s

Historical execution duration stats (from %d completed instances):
- Average: %.2f seconds
- Min: %.2f seconds
- Max: %.2f seconds
- P50: %.2f seconds
- P95: %.2f seconds`,
		task.Name, task.Type, task.TimeoutSeconds, task.MaxRetry, task.RetryPolicy,
		stats.Count, stats.AvgDuration, stats.MinDuration, stats.MaxDuration, stats.P50Duration, stats.P95Duration)

	result, err := llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("llm predict duration: %w", err)
	}

	var parsed struct {
		PredictedDurationSeconds float64 `json:"predicted_duration_seconds"`
		Confidence               float64 `json:"confidence"`
		Trend                    string  `json:"trend"`
		Explanation              string  `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		return nil, fmt.Errorf("parse llm predict output: %w (raw=%s)", err, result)
	}

	return &PredictResponse{
		PredictedDurationSeconds: parsed.PredictedDurationSeconds,
		Confidence:               parsed.Confidence,
		Trend:                    parsed.Trend,
		Explanation:              parsed.Explanation,
		HistoricalStats:          stats,
	}, nil
}
