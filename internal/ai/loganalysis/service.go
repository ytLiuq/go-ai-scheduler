package loganalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
)

// AnalysisResponse contains a heuristic or LLM summary for one execution log.
type AnalysisResponse struct {
	Summary    string   `json:"summary"`
	Severity   string   `json:"severity"`
	Categories []string `json:"categories"`
	RootCause  string   `json:"root_cause,omitempty"`
	Fix        string   `json:"fix,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
}

// Analyze applies deterministic heuristics to one log payload.
func Analyze(logText string) *AnalysisResponse {
	return heuristicAnalyze(logText)
}

// AnalyzeWithLLM runs analysis, preferring LLM when available.
func AnalyzeWithLLM(ctx context.Context, llm *adapter.LLMAdapter, logText, errorCode, taskType string, retryCount int) *AnalysisResponse {
	if llm != nil && llm.Enabled() {
		if resp := llmAnalyze(ctx, llm, logText, errorCode, taskType, retryCount); resp != nil {
			return resp
		}
	}
	return heuristicAnalyze(logText)
}

func llmAnalyze(ctx context.Context, llm *adapter.LLMAdapter, logText, errorCode, taskType string, retryCount int) *AnalysisResponse {
	systemPrompt := `You are an on-call SRE analyzing task failures. Return ONLY valid JSON:
{"summary": "<one-line summary>", "severity": "low|medium|high", "categories": ["<category>"], "root_cause": "<root cause>", "fix": "<actionable fix>", "confidence": <0.0-1.0>}`

	userPrompt := fmt.Sprintf("Task type: %s\nError code: %s\nRetry count: %d\nError log:\n%s",
		taskType, errorCode, retryCount, logText)

	result, err := llm.Complete(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil
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
		return nil
	}
	return &AnalysisResponse{
		Summary:    parsed.Summary,
		Severity:   parsed.Severity,
		Categories: parsed.Categories,
		RootCause:  parsed.RootCause,
		Fix:        parsed.Fix,
		Confidence: parsed.Confidence,
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func heuristicAnalyze(logText string) *AnalysisResponse {
	lower := strings.ToLower(logText)
	response := &AnalysisResponse{
		Summary:    "No obvious failure signal detected.",
		Severity:   "info",
		Categories: []string{"general"},
	}

	switch {
	case strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "timeout"):
		response.Summary = "Execution likely exceeded its timeout budget."
		response.Severity = "high"
		response.Categories = []string{"timeout", "latency"}
	case strings.Contains(lower, "500 internal server error") || strings.Contains(lower, "status=500") || strings.Contains(lower, "status 500"):
		response.Summary = "Upstream HTTP dependency returned a server error."
		response.Severity = "high"
		response.Categories = []string{"http", "upstream"}
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "dial tcp"):
		response.Summary = "Network connectivity or service discovery failure detected."
		response.Severity = "high"
		response.Categories = []string{"network"}
	case strings.Contains(lower, "permission denied"):
		response.Summary = "Execution failed due to missing permission."
		response.Severity = "medium"
		response.Categories = []string{"permission"}
	case strings.Contains(lower, "panic:") || strings.Contains(lower, "fatal"):
		response.Summary = "Application crash signal detected in logs."
		response.Severity = "high"
		response.Categories = []string{"crash"}
	}

	return response
}
