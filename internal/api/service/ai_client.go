package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AIClient calls the ai-service for log analysis.
type AIClient struct {
	httpClient *http.Client
	serviceURL string
}

// NewAIClient creates an AI service client.
func NewAIClient(serviceURL string) *AIClient {
	if serviceURL == "" {
		serviceURL = "http://127.0.0.1:8083"
	}
	return &AIClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		serviceURL: strings.TrimRight(serviceURL, "/"),
	}
}

// AnalyzeLog sends log text to the ai-service for LLM analysis.
func (c *AIClient) AnalyzeLog(ctx context.Context, logText, errorCode, taskType string, retryCount int, instanceID int64) (*LogAnalysisResult, error) {
	reqBody := map[string]interface{}{
		"log":         logText,
		"error_code":  errorCode,
		"task_type":   taskType,
		"retry_count": retryCount,
	}
	if instanceID > 0 {
		reqBody["instance_id"] = instanceID
	}
	body, _ := json.Marshal(reqBody)

	url := c.serviceURL + "/api/v1/log-analysis/analyze"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build ai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai service request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ai service returned %d", resp.StatusCode)
	}

	var result LogAnalysisResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse ai response: %w", err)
	}
	return &result, nil
}

// LogAnalysisResult mirrors loganalysis.AnalysisResponse from the ai-service.
type LogAnalysisResult struct {
	Summary    string   `json:"summary"`
	Severity   string   `json:"severity"`
	Categories []string `json:"categories"`
	RootCause  string   `json:"root_cause,omitempty"`
	Fix        string   `json:"fix,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
}
