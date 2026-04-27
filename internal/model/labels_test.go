package model

import "testing"

func TestMatchLabels(t *testing.T) {
	workerLabels := map[string]string{"env": "prod", "zone": "us-east", "host": "w1"}

	if !MatchLabels(workerLabels, nil) {
		t.Fatal("expected nil selector to match")
	}
	if !MatchLabels(workerLabels, map[string]string{}) {
		t.Fatal("expected empty selector to match")
	}
	if !MatchLabels(workerLabels, map[string]string{"env": "prod"}) {
		t.Fatal("expected single label match")
	}
	if !MatchLabels(workerLabels, map[string]string{"env": "prod", "zone": "us-east"}) {
		t.Fatal("expected multi label match")
	}
	if MatchLabels(workerLabels, map[string]string{"env": "staging"}) {
		t.Fatal("expected mismatch on value")
	}
	if MatchLabels(workerLabels, map[string]string{"env": "prod", "region": "x"}) {
		t.Fatal("expected mismatch on extra key")
	}
}

func TestDecodeLabels(t *testing.T) {
	m := DecodeLabels(`{"env":"prod","zone":"us-east"}`)
	if len(m) != 2 || m["env"] != "prod" || m["zone"] != "us-east" {
		t.Fatalf("unexpected decode: %v", m)
	}

	if m2 := DecodeLabels("{}"); m2 != nil {
		t.Fatalf("expected nil for empty JSON: %v", m2)
	}

	if m3 := DecodeLabels(""); m3 != nil {
		t.Fatalf("expected nil for empty string: %v", m3)
	}
}
