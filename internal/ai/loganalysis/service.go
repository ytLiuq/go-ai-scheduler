package loganalysis

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

// AnalysisResponse contains an LLM summary for one execution log.
type AnalysisResponse struct {
	Summary    string   `json:"summary"`
	Severity   string   `json:"severity"`
	Categories []string `json:"categories"`
	RootCause  string   `json:"root_cause,omitempty"`
	Fix        string   `json:"fix,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
}

// AnalyzeWithLLM runs log analysis via LLM. Returns ErrLLMRequired if no LLM
// adapter is configured.
func AnalyzeWithLLM(ctx context.Context, llm *adapter.LLMAdapter, logText, errorCode, taskType string, retryCount int) (*AnalysisResponse, error) {
	if llm == nil || !llm.Enabled() {
		return nil, ErrLLMRequired
	}

	systemPrompt := prompts.LogAnalysis

	userPrompt := fmt.Sprintf("Task type: %s\nError code: %s\nRetry count: %d\nError log:\n%s",
		taskType, errorCode, retryCount, logText)

	result, err := llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("llm log analysis: %w", err)
	}

	type llmResp struct {
		Summary    string   `json:"summary"`
		Severity   string   `json:"severity"`
		Categories []string `json:"categories"`
		RootCause  string   `json:"root_cause"`
		Fix        string   `json:"fix"`
		Confidence float64  `json:"confidence"`
	}
	var parsed llmResp
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		return nil, fmt.Errorf("parse llm log analysis output: %w (raw=%s)", err, result)
	}
	return &AnalysisResponse{
		Summary:    parsed.Summary,
		Severity:   parsed.Severity,
		Categories: parsed.Categories,
		RootCause:  parsed.RootCause,
		Fix:        parsed.Fix,
		Confidence: parsed.Confidence,
	}, nil
}
