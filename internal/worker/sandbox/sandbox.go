package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Config defines sandbox constraints for a single task run.
type Config struct {
	WorkDir        string
	MaxMemoryBytes int64
	MaxCPUPercent  int // 1-100, or 0 to disable
	Timeout        time.Duration
}

// Sandbox isolates task execution with a dedicated working directory and
// optional resource limits.
type Sandbox struct {
	workDir string
	config  Config
}

// New creates a sandbox with a unique working directory under baseDir.
func New(baseDir string, cfg Config) (*Sandbox, error) {
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	workDir, err := os.MkdirTemp(baseDir, "task-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("create sandbox workdir: %w", err)
	}
	cfg.WorkDir = workDir
	return &Sandbox{workDir: workDir, config: cfg}, nil
}

// WorkDir returns the sandbox working directory.
func (s *Sandbox) WorkDir() string {
	return s.workDir
}

// ShellExec runs a shell command inside the sandbox.
// extraEnv pairs are added to the isolated environment.
func (s *Sandbox) ShellExec(ctx context.Context, command string, extraEnv map[string]string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = s.workDir

	// Environment isolation: strip dangerous variables, keep PATH.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + s.workDir,
		"USER=nobody",
		"TMPDIR=" + s.workDir,
		"SANDBOX=true",
	}
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Detach the child from the parent process group so it can be killed
	// independently without affecting the worker.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Apply cgroup-based resource limits if configured.
	applyResourceLimits(cmd, s.config)

	return cmd.CombinedOutput()
}

// Cleanup removes the sandbox working directory and all its contents.
func (s *Sandbox) Cleanup() error {
	if s.workDir == "" {
		return nil
	}
	return os.RemoveAll(s.workDir)
}
