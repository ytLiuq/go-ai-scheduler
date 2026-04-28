package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LLMAdapter provides a unified interface to LLM providers (OpenAI-compatible API).
type LLMAdapter struct {
	endpoint   string
	apiKey     string
	model      string
	httpClient *http.Client
}

// Config configures the LLM adapter.
type Config struct {
	Endpoint string // API base URL, e.g. https://api.openai.com/v1
	APIKey   string
	Model    string // e.g. gpt-4o, claude-sonnet-4-6
	Timeout  time.Duration
}

// New creates an LLM adapter. Returns nil if no endpoint is configured.
func New(cfg Config) *LLMAdapter {
	if cfg.Endpoint == "" {
		return nil
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &LLMAdapter{
		endpoint:   cfg.Endpoint,
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// Enabled reports whether the adapter is configured.
func (a *LLMAdapter) Enabled() bool {
	return a != nil && a.endpoint != ""
}

// ChatRequest is an OpenAI-compatible chat completion request.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Message is a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is an OpenAI-compatible chat completion response.
type ChatResponse struct {
	Choices []Choice `json:"choices"`
}

// Choice contains the completion content.
type Choice struct {
	Message Message `json:"message"`
}

// Complete sends a chat completion request and returns the response text.
// Falls back to empty string on error (AI is never critical-path).
func (a *LLMAdapter) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if !a.Enabled() {
		return "", fmt.Errorf("llm adapter not configured")
	}

	reqBody := ChatRequest{
		Model: a.model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := a.endpoint + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("parse llm response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm returned empty choices")
	}
	return chatResp.Choices[0].Message.Content, nil
}
