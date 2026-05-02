package taskparser

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/ai/prompts"
	"github.com/example/go-ai-scheduler/internal/pkg/cronexpr"
)

// ErrLLMRequired is returned when an LLM adapter is not configured.
var ErrLLMRequired = fmt.Errorf("llm adapter not configured or disabled")

// TaskParseResponse is the structured output from natural language task parsing.
type TaskParseResponse struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Image      string  `json:"image,omitempty"`
	CronExpr   string  `json:"cron_expr,omitempty"`
	Payload    string  `json:"payload,omitempty"`
	MaxRetry   int     `json:"max_retry"`
	RetryPolicy string `json:"retry_policy,omitempty"`
	Explanation string  `json:"explanation"`
	Confidence float64 `json:"confidence"`
}

// ParseNaturalLanguage uses LLM to convert a natural language description into
// a pre-filled task configuration.
func ParseNaturalLanguage(ctx context.Context, llm *adapter.LLMAdapter, input string) (*TaskParseResponse, error) {
	if llm == nil || !llm.Enabled() {
		return nil, ErrLLMRequired
	}

	systemPrompt := prompts.TaskParser

	result, err := llm.Complete(ctx, systemPrompt, input)
	if err != nil {
		return nil, fmt.Errorf("llm task parse: %w", err)
	}

	var resp TaskParseResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return nil, fmt.Errorf("parse llm task output: %w (raw=%s)", err, result)
	}
	if resp.Name == "" {
		return nil, fmt.Errorf("llm returned empty task name")
	}
	if resp.CronExpr != "" {
		if _, valErr := cronexpr.NextAfter(time.Now(), resp.CronExpr); valErr != nil {
			return nil, fmt.Errorf("llm returned invalid cron expression %q: %w", resp.CronExpr, valErr)
		}
	}
	return &resp, nil
}
