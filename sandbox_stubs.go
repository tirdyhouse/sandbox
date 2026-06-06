//go:build !linux && !darwin

package sandbox

// probeLinux is a stub for non-Linux, non-Darwin platforms.
func probeLinux() ProbeResult {
	return ProbeResult{Platform: "linux", Backend: "none", Warning: "not running on Linux"}
}

// probeDarwin is a stub for non-Linux, non-Darwin platforms.
func probeDarwin() ProbeResult {
	return ProbeResult{Platform: "darwin", Backend: "none", Warning: "not running on macOS"}
}
