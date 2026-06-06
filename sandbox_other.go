//go:build !linux && !darwin

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

func probeResult() ProbeResult {
	// Not called on this platform — Probe() handles the fallback directly.
	return ProbeResult{}
}

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	// No sandbox enforcement. The command runs as-is.
	// Callers can check Available() to decide whether to proceed.
	return nil
}
