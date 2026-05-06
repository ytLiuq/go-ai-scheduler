package worker

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewSandbox(t *testing.T) {
	sb, err := NewSandbox("", SandboxConfig{})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	defer sb.Cleanup()

	if sb.WorkDir() == "" {
		t.Fatal("work dir should not be empty")
	}
	if !strings.Contains(sb.WorkDir(), "task-sandbox-") {
		t.Fatalf("unexpected work dir: %s", sb.WorkDir())
	}
	// Verify directory exists.
	if _, err := os.Stat(sb.WorkDir()); os.IsNotExist(err) {
		t.Fatal("work dir should exist")
	}
}

func TestSandboxCleanup(t *testing.T) {
	sb, err := NewSandbox("", SandboxConfig{})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	dir := sb.WorkDir()
	if err := sb.Cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("work dir should be removed after cleanup")
	}
}

func TestSandboxShellExec(t *testing.T) {
	sb, err := NewSandbox("", SandboxConfig{})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	defer sb.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := sb.ShellExec(ctx, "echo hello", nil)
	if err != nil {
		t.Fatalf("shell exec: %v, output=%s", err, string(out))
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("expected 'hello' in output, got %s", string(out))
	}
}

func TestSandboxShellExecWithEnv(t *testing.T) {
	sb, err := NewSandbox("", SandboxConfig{})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	defer sb.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := sb.ShellExec(ctx, "echo $MY_VAR", map[string]string{"MY_VAR": "test_value"})
	if err != nil {
		t.Fatalf("shell exec: %v", err)
	}
	if !strings.Contains(string(out), "test_value") {
		t.Fatalf("expected env var in output, got %s", string(out))
	}
}

func TestSandboxShellExecTimeout(t *testing.T) {
	sb, err := NewSandbox("", SandboxConfig{})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	defer sb.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = sb.ShellExec(ctx, "sleep 5", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestSandboxCustomBaseDir(t *testing.T) {
	tmp := os.TempDir()
	sb, err := NewSandbox(tmp, SandboxConfig{})
	if err != nil {
		t.Fatalf("new sandbox: %v", err)
	}
	defer sb.Cleanup()

	if !strings.HasPrefix(sb.WorkDir(), tmp) {
		t.Fatalf("work dir should be under base dir: %s", sb.WorkDir())
	}
}
