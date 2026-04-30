package taskparser

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
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

	systemPrompt := `You are a task scheduler assistant. Convert the user's natural language description into a task configuration. Return ONLY valid JSON in this exact format:
{
  "name": "<short kebab-case task name>",
  "type": "container|shell|http",
  "image": "<docker image, for container type only>",
  "cron_expr": "<5-field cron, empty if not periodic>",
  "payload": "<command or args, empty if not needed>",
  "max_retry": <integer, default 0>,
  "retry_policy": "fixed_interval|exponential_backoff|error_code",
  "explanation": "<one-line summary of what you understood>",
  "confidence": <0.0-1.0>
}

Rules:
- If it's a container image (has slashes or common registries), use type "container"
- If it mentions HTTP/URL, use type "http"
- Otherwise default to type "shell"
- For cron: "每天早上9点" = "0 9 * * *", "每小时" = "0 * * * *", "每分钟" = "* * * * *", "工作日" = weekdays, etc.
- If no schedule mentioned, cron_expr should be ""
- If retry is mentioned (e.g., "重试3次"), set max_retry accordingly`

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
