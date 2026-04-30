package loganalysis

import "testing"

func TestAnalyzeWithLLMRequiresLLM(t *testing.T) {
	_, err := AnalyzeWithLLM(t.Context(), nil, "connection refused", "conn_refused", "http", 0)
	if err == nil {
		t.Fatal("expected ErrLLMRequired when no LLM adapter")
	}
	if err != ErrLLMRequired {
		t.Fatalf("expected ErrLLMRequired, got %v", err)
	}
}
