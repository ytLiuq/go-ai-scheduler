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

