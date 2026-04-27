package loganalysis

import "strings"

// AnalysisResponse contains a lightweight heuristic summary for one execution log payload.
type AnalysisResponse struct {
	Summary    string   `json:"summary"`
	Severity   string   `json:"severity"`
	Categories []string `json:"categories"`
}

// Analyze applies deterministic heuristics to one log payload.
func Analyze(logText string) *AnalysisResponse {
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
