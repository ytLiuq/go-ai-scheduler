//go:build !linux

package worker

import "os/exec"

// applyResourceLimits is a no-op on non-Linux platforms.
func applyResourceLimits(_ *exec.Cmd, _ Config) {}
