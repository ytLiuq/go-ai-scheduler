package adapter

import (
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
