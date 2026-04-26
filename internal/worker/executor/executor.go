package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

// Execute runs one task based on task type.
func Execute(ctx context.Context, taskType string, payload string) error {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case "shell":
		cmd := exec.CommandContext(ctx, "bash", "-lc", payload)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("shell execute failed: %w output=%s", err, string(output))
		}
		return nil
	case "http":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, payload, nil)
		if err != nil {
			return fmt.Errorf("build http request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("http execute failed: %w", err)
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode >= 300 {
			return fmt.Errorf("http execute status=%s", resp.Status)
		}
		return nil
	default:
		return fmt.Errorf("unsupported task type: %s", taskType)
	}
}

