package worker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const (
	cgroupRoot    = "/sys/fs/cgroup"
	taskCgroupDir = "go-ai-scheduler"
)

// applyResourceLimits configures cgroups v2 limits for the command.
// Falls back silently if cgroups are unavailable.
func applyResourceLimits(cmd *exec.Cmd, cfg SandboxConfig) {
	cgPath := filepath.Join(cgroupRoot, taskCgroupDir)
	if err := os.MkdirAll(cgPath, 0755); err != nil {
		return // cgroups not available
	}

	pid := strconv.Itoa(os.Getpid())

	// Memory limit.
	if cfg.MaxMemoryBytes > 0 {
		memFile := filepath.Join(cgPath, "memory.max")
		_ = os.WriteFile(memFile, []byte(strconv.FormatInt(cfg.MaxMemoryBytes, 10)), 0644)
		// Add current process to cgroup.
		_ = os.WriteFile(filepath.Join(cgPath, "cgroup.procs"), []byte(pid), 0644)
	}

	// CPU limit via cpu.max (quota period).
	if cfg.MaxCPUPercent > 0 && cfg.MaxCPUPercent < 100 {
		// quota_us = (percentage / 100) * period_us
		period := 100000 // 100ms
		quota := period * cfg.MaxCPUPercent / 100
		cpuFile := filepath.Join(cgPath, "cpu.max")
		_ = os.WriteFile(cpuFile, []byte(strconv.Itoa(quota)+" "+strconv.Itoa(period)), 0644)
	}
}
