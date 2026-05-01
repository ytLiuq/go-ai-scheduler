package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	Endpoint string
	APIKey   string
	Model    string
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

// Endpoint returns the configured API base URL.
func (a *LLMAdapter) Endpoint() string {
	if a == nil {
		return ""
	}
	return a.endpoint
}

// Model returns the configured model name.
func (a *LLMAdapter) Model() string {
	if a == nil {
		return ""
	}
	return a.model
}

// HasAPIKey reports whether a non-empty API key is configured.
func (a *LLMAdapter) HasAPIKey() bool {
	return a != nil && a.apiKey != ""
}

// --------------- types ---------------

// ChatRequest is an OpenAI-compatible chat completion request.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream,omitempty"`
}

// Message is a chat message.
type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

// Tool is an OpenAI function-calling tool definition.
type Tool struct {
	Type     string           `json:"type"`
	Function FunctionDef      `json:"function"`
}

// FunctionDef describes a callable function.
type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall is an LLM-requested function invocation.
type ToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function FunctionCallArgs `json:"function"`
}

// FunctionCallArgs holds the function name and JSON-encoded arguments.
type FunctionCallArgs struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatResponse is an OpenAI-compatible non-streaming response.
type ChatResponse struct {
	Choices []Choice `json:"choices"`
}

// Choice contains the completion content.
type Choice struct {
	Message      Message  `json:"message"`
	FinishReason string   `json:"finish_reason"`
}

// --------------- streaming types ---------------

// StreamEvent is emitted during streaming chat completion.
type StreamEvent struct {
	DeltaContent     string    // incremental text
	ReasoningContent string    // accumulated reasoning (DeepSeek thinking mode)
	ToolCalls        []ToolCall // accumulated tool calls (only on finish_reason=tool_calls)
	FinishReason     string
	Error            error
}

// chatChunk is a single SSE data line parsed from the stream.
type chatChunk struct {
	Choices []chunkChoice `json:"choices"`
}

type chunkChoice struct {
	Index        int         `json:"index"`
	Delta        chunkDelta  `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type chunkDelta struct {
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

// --------------- non-streaming (kept for backward compat) ---------------

// Complete sends a chat completion request and returns the response text.
func (a *LLMAdapter) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if !a.Enabled() {
		return "", fmt.Errorf("llm adapter not configured")
	}

	msgs := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	return a.completeMessages(ctx, msgs)
}

// CompleteWithTools sends a non-streaming request with tools; returns content + tool calls.
func (a *LLMAdapter) CompleteWithTools(ctx context.Context, messages []Message, tools []Tool) (string, []ToolCall, error) {
	if !a.Enabled() {
		return "", nil, fmt.Errorf("llm adapter not configured")
	}

	reqBody := ChatRequest{
		Model:    a.model,
		Messages: messages,
		Tools:    tools,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}

	url := a.endpoint + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("llm returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", nil, fmt.Errorf("parse llm response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", nil, fmt.Errorf("llm returned empty choices")
	}
	choice := chatResp.Choices[0]
	return choice.Message.Content, choice.Message.ToolCalls, nil
}

// --------------- streaming ---------------

// CompleteStream sends a streaming chat completion request. It returns a channel
// that emits events as chunks arrive, and is closed when the stream ends.
func (a *LLMAdapter) CompleteStream(ctx context.Context, messages []Message, tools []Tool) <-chan StreamEvent {
	ch := make(chan StreamEvent, 8)
	go a.runStream(ctx, ch, messages, tools)
	return ch
}

// CompleteStreamWithClient uses the caller's HTTP client (for longer timeouts).
func (a *LLMAdapter) CompleteStreamWithClient(ctx context.Context, client *http.Client, messages []Message, tools []Tool) <-chan StreamEvent {
	ch := make(chan StreamEvent, 8)
	go a.runStreamWithClient(ctx, client, ch, messages, tools)
	return ch
}

func (a *LLMAdapter) runStream(ctx context.Context, ch chan<- StreamEvent, messages []Message, tools []Tool) {
	a.runStreamWithClient(ctx, a.httpClient, ch, messages, tools)
}

func (a *LLMAdapter) runStreamWithClient(ctx context.Context, client *http.Client, ch chan<- StreamEvent, messages []Message, tools []Tool) {
	defer close(ch)

	if !a.Enabled() {
		ch <- StreamEvent{Error: fmt.Errorf("llm adapter not configured")}
		return
	}

	reqBody := ChatRequest{
		Model:    a.model,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		ch <- StreamEvent{Error: err}
		return
	}

	url := a.endpoint + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		ch <- StreamEvent{Error: err}
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		ch <- StreamEvent{Error: fmt.Errorf("llm stream request: %w", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		ch <- StreamEvent{Error: fmt.Errorf("llm returned %d: %s", resp.StatusCode, string(body))}
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	// Accumulate tool calls across chunks.
	accum := make(map[int]*ToolCall)
	var reasoningAccum strings.Builder

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			ch <- StreamEvent{Error: err}
			return
		}

		line := scanner.Text()
		if line == "" || line == "data: [DONE]" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed lines
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Accumulate tool calls if present.
		for _, tc := range delta.ToolCalls {
			if existing, ok := accum[tc.Index]; ok {
				if tc.Function.Arguments != "" {
					existing.Function.Arguments += tc.Function.Arguments
				}
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				if tc.ID != "" {
					existing.ID = tc.ID
				}
			} else {
				accum[tc.Index] = &ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: FunctionCallArgs{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		// Accumulate reasoning content (DeepSeek thinking mode).
		if delta.ReasoningContent != "" {
			reasoningAccum.WriteString(delta.ReasoningContent)
		}

		// Emit delta content.
		if delta.Content != "" {
			ch <- StreamEvent{DeltaContent: delta.Content}
		}

		// On finish, emit final event with tool calls if any.
		if choice.FinishReason != "" {
			ev := StreamEvent{FinishReason: choice.FinishReason}
			if len(accum) > 0 {
				ev.ToolCalls = make([]ToolCall, 0, len(accum))
				for i := 0; i < len(accum); i++ {
					if tc, ok := accum[i]; ok {
						ev.ToolCalls = append(ev.ToolCalls, *tc)
					}
				}
			}
			if reasoningAccum.Len() > 0 {
				ev.ReasoningContent = reasoningAccum.String()
			}
			ch <- ev
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Error: err}
	}
}

func (a *LLMAdapter) completeMessages(ctx context.Context, messages []Message) (string, error) {
	reqBody := ChatRequest{
		Model:    a.model,
		Messages: messages,
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
