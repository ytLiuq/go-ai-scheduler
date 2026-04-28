package loganalysis

import "testing"

func TestAnalyzeTimeout(t *testing.T) {
	resp := Analyze("context deadline exceeded after 30 seconds")
	if resp.Severity != "high" {
		t.Fatalf("expected high severity for timeout, got %s", resp.Severity)
	}
	if len(resp.Categories) == 0 || resp.Categories[0] != "timeout" {
		t.Fatalf("expected timeout category, got %v", resp.Categories)
	}
}

func TestAnalyzeHTTP500(t *testing.T) {
	resp := Analyze("request failed: status=500 Internal Server Error")
	if resp.Severity != "high" || resp.Categories[0] != "http" {
		t.Fatalf("expected http upstream category, got severity=%s categories=%v", resp.Severity, resp.Categories)
	}
}

func TestAnalyzeConnectionRefused(t *testing.T) {
	resp := Analyze("dial tcp 1.2.3.4:8080: connection refused")
	if resp.Categories[0] != "network" {
		t.Fatalf("expected network category, got %v", resp.Categories)
	}
}

func TestAnalyzePermission(t *testing.T) {
	resp := Analyze("permission denied accessing /etc/secret")
	if resp.Severity != "medium" {
		t.Fatalf("expected medium severity, got %s", resp.Severity)
	}
	if resp.Categories[0] != "permission" {
		t.Fatalf("expected permission category, got %v", resp.Categories)
	}
}

func TestAnalyzePanic(t *testing.T) {
	resp := Analyze("panic: runtime error: index out of range")
	if resp.Severity != "high" || resp.Categories[0] != "crash" {
		t.Fatalf("expected crash category, got severity=%s categories=%v", resp.Severity, resp.Categories)
	}
}

func TestAnalyzeNormal(t *testing.T) {
	resp := Analyze("task completed successfully in 1.2s")
	if resp.Severity != "info" {
		t.Fatalf("expected info severity, got %s", resp.Severity)
	}
}

func TestAnalyzeWithLLMFallback(t *testing.T) {
	// No LLM — should fall back to heuristic.
	resp := AnalyzeWithLLM(t.Context(), nil, "connection refused", "conn_refused", "http", 0)
	if resp.Severity != "high" || resp.Categories[0] != "network" {
		t.Fatalf("expected network, got severity=%s categories=%v", resp.Severity, resp.Categories)
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("hello world", "hello") {
		t.Fatal("expected match")
	}
	if containsAny("hello world", "goodbye") {
		t.Fatal("expected no match")
	}
	if !containsAny("hello world", "foo", "world") {
		t.Fatal("expected match on second sub")
	}
}
