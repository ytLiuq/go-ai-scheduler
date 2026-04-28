package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
)

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
// cron expression. Falls back to a heuristic for simple patterns if LLM is
// unavailable.
func ParseNaturalLanguage(ctx context.Context, llm *adapter.LLMAdapter, input string) (*ParseNaturalResponse, error) {
	if llm != nil && llm.Enabled() {
		return parseWithLLM(ctx, llm, input)
	}
	return parseHeuristic(input)
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

// parseHeuristic handles common patterns without LLM.
func parseHeuristic(input string) (*ParseNaturalResponse, error) {
	heuristics := map[string]string{
		"every hour":        "0 * * * *",
		"every minute":      "* * * * *",
		"every day":         "0 0 * * *",
		"every monday":      "0 0 * * 1",
		"every weekday":     "0 0 * * 1-5",
		"every weekend":     "0 0 * * 6,0",
		"every 5 minutes":   "*/5 * * * *",
		"every 15 minutes":  "*/15 * * * *",
		"every 30 minutes":  "*/30 * * * *",
		"every monday 9am":  "0 9 * * 1",
		"every day at noon": "0 12 * * *",
		"every day at midnight": "0 0 * * *",
	}

	for pattern, expr := range heuristics {
		if contains(input, pattern) {
			return &ParseNaturalResponse{
				CronExpression: expr,
				Explanation:    fmt.Sprintf("matched heuristic pattern: %s -> %s", pattern, expr),
				Confidence:     0.8,
			}, nil
		}
	}
	return nil, fmt.Errorf("unable to parse natural language: %q (try LLM or explicit cron syntax)", input)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
