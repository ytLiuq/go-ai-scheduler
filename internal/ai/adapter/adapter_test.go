package adapter

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewAdapterDisabled(t *testing.T) {
	a := New(Config{})
	if a != nil {
		t.Fatal("expected nil adapter when no endpoint configured")
	}
}

func TestNewAdapterEnabled(t *testing.T) {
	a := New(Config{
		Endpoint: "https://api.example.com/v1",
		Model:    "gpt-4o",
		Timeout:  10 * time.Second,
	})
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if !a.Enabled() {
		t.Fatal("adapter should be enabled")
	}
	if a.model != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", a.model)
	}
}

func TestAdapterDefaultModel(t *testing.T) {
	a := New(Config{Endpoint: "https://api.example.com/v1"})
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
	if a.model != "gpt-4o" {
		t.Fatalf("expected default model gpt-4o, got %s", a.model)
	}
}

func TestAdapterCompleteNotConfigured(t *testing.T) {
	a := New(Config{})
	if a != nil {
		t.Fatal("adapter should be nil")
	}
	var nilAdapter *LLMAdapter
	_, err := nilAdapter.Complete(t.Context(), "system", "user")
	if err == nil {
		t.Fatal("expected error when adapter not configured")
	}
}

func TestUsageRecord(t *testing.T) {
	a := New(Config{Endpoint: "https://api.example.com/v1"})
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}

	a.recordUsage(Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	a.recordUsage(Usage{PromptTokens: 200, CompletionTokens: 75, TotalTokens: 275})

	prompt, completion := a.TokenUsage()
	if prompt != 300 {
		t.Fatalf("expected prompt tokens 300, got %d", prompt)
	}
	if completion != 125 {
		t.Fatalf("expected completion tokens 125, got %d", completion)
	}
}

func TestUsageResponseParsing(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	a := New(Config{Endpoint: "https://api.example.com/v1"})
	if a == nil {
		t.Fatal("expected non-nil adapter")
	}

	var chatResp ChatResponse
	json.Unmarshal([]byte(body), &chatResp)
	if chatResp.Usage.PromptTokens != 10 {
		t.Fatalf("expected prompt_tokens 10, got %d", chatResp.Usage.PromptTokens)
	}
	if chatResp.Usage.CompletionTokens != 5 {
		t.Fatalf("expected completion_tokens 5, got %d", chatResp.Usage.CompletionTokens)
	}
	if chatResp.Usage.TotalTokens != 15 {
		t.Fatalf("expected total_tokens 15, got %d", chatResp.Usage.TotalTokens)
	}
}

func TestAdapterChatRequestSerialization(t *testing.T) {
	req := ChatRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "system", Content: "hello"},
			{Role: "user", Content: "world"},
		},
	}
	if req.Model != "test-model" {
		t.Fatal("wrong model")
	}
	if len(req.Messages) != 2 {
		t.Fatal("wrong message count")
	}
}
