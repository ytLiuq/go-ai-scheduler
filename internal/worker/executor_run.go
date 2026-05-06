package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

// Execute runs one task based on task type. extraEnv is injected as environment
// variables for shell tasks (idempotency key, shard info, etc.).
func Execute(ctx context.Context, taskType string, payload string, image string, extraEnv map[string]string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case "shell":
		cmd := exec.CommandContext(ctx, "bash", "-lc", payload)
		cmd.Env = append(cmd.Env,
			"IDEMPOTENCY_KEY="+getEnv(extraEnv, "IDEMPOTENCY_KEY"),
			"SHARD_NO="+getEnv(extraEnv, "SHARD_NO"),
			"SHARD_TOTAL="+getEnv(extraEnv, "SHARD_TOTAL"),
			"SCHEDULE_INSTANCE_ID="+getEnv(extraEnv, "SCHEDULE_INSTANCE_ID"),
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output), fmt.Errorf("shell execute failed: %w output=%s", err, string(output))
		}
		return string(output), nil

	case "http":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, payload, nil)
		if err != nil {
			return "", fmt.Errorf("build http request: %w", err)
		}
		if key := extraEnv["IDEMPOTENCY_KEY"]; key != "" {
			req.Header.Set("X-Idempotency-Key", key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("http execute failed: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return string(body), fmt.Errorf("http execute status=%s", resp.Status)
		}
		return string(body), nil

	case "container":
		if image == "" {
			return "", fmt.Errorf("container task requires a non-empty image")
		}
		args := []string{"run", "--rm"}
		if payload != "" {
			args = append(args, strings.Fields(payload)...)
		}
		args = append(args, image)
		cmd := exec.CommandContext(ctx, "docker", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output), fmt.Errorf("container execute failed: %w output=%s", err, string(output))
		}
		return string(output), nil

	default:
		return "", fmt.Errorf("unsupported task type: %s", taskType)
	}
}

func getEnv(m map[string]string, key string) string {
	if m == nil {
		return ""
	}
	return m[key]
}
