package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
)

// ErrLLMRequired is returned when an LLM is not available and the operation
// cannot fall back to a heuristic implementation.
var ErrLLMRequired = fmt.Errorf("llm adapter not configured or disabled")

// NextRunResponse describes the next scheduled run for one cron expression.
type NextRunResponse struct {
	Expression string    `json:"expression"`
	BaseTime   time.Time `json:"base_time"`
	NextRun    time.Time `json:"next_run"`
}

// ParseNaturalResponse is the structured output for natural language parsing.
type ParseNaturalResponse struct {
	CronExpression string  `json:"cron_expression"`
	Explanation    string  `json:"explanation"`
	Confidence     float64 `json:"confidence"`
}

// NextRun computes the next run strictly after baseTime.
func NextRun(expression string, baseTime time.Time) (*NextRunResponse, error) {
	if baseTime.IsZero() {
		baseTime = time.Now()
	}
	nextRun, err := cronexpr.NextAfter(baseTime, expression)
	if err != nil {
		return nil, err
	}
	return &NextRunResponse{
		Expression: expression,
		BaseTime:   baseTime,
		NextRun:    nextRun,
	}, nil
}

// ParseNaturalLanguage uses LLM to convert a natural language description to a
// cron expression. Returns ErrLLMRequired if no LLM adapter is configured.
func ParseNaturalLanguage(ctx context.Context, llm *adapter.LLMAdapter, input string) (*ParseNaturalResponse, error) {
	if llm == nil || !llm.Enabled() {
		return nil, ErrLLMRequired
	}
	return parseWithLLM(ctx, llm, input)
}

func parseWithLLM(ctx context.Context, llm *adapter.LLMAdapter, input string) (*ParseNaturalResponse, error) {
	systemPrompt := `You are a cron expression generator. Convert the user's natural language description into a standard 5-field cron expression (minute hour day-of-month month day-of-week). Return ONLY valid JSON in this exact format:
{"cron_expression": "<5-field cron>", "explanation": "<one-line explanation>", "confidence": <0.0-1.0>}`

	result, err := llm.Complete(ctx, systemPrompt, input)
	if err != nil {
		return nil, fmt.Errorf("llm cron parse: %w", err)
	}

	var resp ParseNaturalResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("parse llm cron output: %w (raw=%s)", err, result)
	}
	if resp.CronExpression == "" {
		return nil, fmt.Errorf("llm returned empty cron expression")
	}
	// Validate the expression.
	if _, valErr := cronexpr.NextAfter(time.Now(), resp.CronExpression); valErr != nil {
		return nil, fmt.Errorf("llm returned invalid cron expression %q: %w", resp.CronExpression, valErr)
	}
	return &resp, nil
}
