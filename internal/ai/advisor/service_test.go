package advisor

import "testing"

func TestGenerateRequiresLLM(t *testing.T) {
	_, err := Generate(t.Context(), nil, Context{
		OnlineWorkers:    1,
		TotalWorkers:     3,
		MaxPendingConfig: 1000,
	})
	if err == nil {
		t.Fatal("expected ErrLLMRequired when no LLM adapter")
	}
	if err != ErrLLMRequired {
		t.Fatalf("expected ErrLLMRequired, got %v", err)
	}
}
