//go:build !linux && !darwin && !windows

package sandbox

import (
	"os/exec"
	"runtime"
)

// On unsupported platforms, sandboxing is not available and all commands
// run unsandboxed. Callers should check Available() or Probe() to
// determine if sandbox enforcement is active.

func available() bool { return false }

func reasonUnavailable() string {
	return "sandboxing not supported on " + runtime.GOOS
}

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	// No sandbox enforcement. The command runs as-is.
	return nil
}
