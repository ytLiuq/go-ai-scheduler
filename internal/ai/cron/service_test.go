package cron

import (
	"strings"
	"testing"
	"time"
)

func TestNextRunValid(t *testing.T) {
	resp, err := NextRun("*/5 * * * *", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("next run: %v", err)
	}
	if resp.Expression != "*/5 * * * *" {
		t.Fatal("wrong expression")
	}
	if resp.NextRun.IsZero() {
		t.Fatal("next run should not be zero")
	}
}

func TestNextRunInvalidExpr(t *testing.T) {
	_, err := NextRun("not-a-cron", time.Now())
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestParseHeuristic(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"run every hour", "0 * * * *"},
		{"every 5 minutes please", "*/5 * * * *"},
		{"every day at midnight", "0 0 * * *"},
		{"every monday 9am", "0 9 * * 1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			resp, err := parseHeuristic(tt.input)
			if err != nil {
				t.Fatalf("parse heuristic: %v", err)
			}
			if resp.CronExpression != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, resp.CronExpression)
			}
			if resp.Confidence < 0.5 {
				t.Fatalf("confidence too low: %f", resp.Confidence)
			}
		})
	}
}

func TestParseHeuristicUnknown(t *testing.T) {
	_, err := parseHeuristic("do something complicated every other tuesday at tea time")
	if err == nil {
		t.Fatal("expected error for unknown pattern")
	}
}

func TestParseNaturalLanguageHeuristicFallback(t *testing.T) {
	// No LLM adapter — should fall back to heuristic.
	resp, err := ParseNaturalLanguage(t.Context(), nil, "every hour")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.CronExpression != "0 * * * *" {
		t.Fatalf("expected 0 * * * *, got %s", resp.CronExpression)
	}
}

func TestContainsFunc(t *testing.T) {
	if !strings.Contains("every hour run", "every hour") {
		t.Fatal("expected contains match")
	}
	if strings.Contains("every min run", "every hour") {
		t.Fatal("expected no match")
	}
}
