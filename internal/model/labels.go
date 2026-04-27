package model

import (
	"encoding/json"
)

// EncodeLabels stores worker labels as JSON for the current bootstrap phase.
func EncodeLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	data, err := json.Marshal(labels)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// DecodeLabels parses a JSON labels string back into a map.
func DecodeLabels(raw string) map[string]string {
	if raw == "" || raw == "{}" {
		return nil
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		return nil
	}
	return labels
}

// MatchLabels returns true if workerLabels contains all keys and values from selectorLabels.
func MatchLabels(workerLabels, selectorLabels map[string]string) bool {
	for k, v := range selectorLabels {
		if wv, ok := workerLabels[k]; !ok || wv != v {
			return false
		}
	}
	return true
}

