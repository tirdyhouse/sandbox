//go:build !linux && !darwin && !windows

package sandbox

// probeLinux is a stub for non-Linux, non-Darwin, non-Windows platforms.
func probeLinux() ProbeResult {
	return ProbeResult{Platform: "linux", Backend: "none", Warning: "not running on Linux"}
}

// probeDarwin is a stub for non-Linux, non-Darwin, non-Windows platforms.
func probeDarwin() ProbeResult {
	return ProbeResult{Platform: "darwin", Backend: "none", Warning: "not running on macOS"}
}

// probeWindows is a stub for non-Windows platforms.
func probeWindows() ProbeResult {
	return ProbeResult{Platform: "windows", Backend: "none", Warning: "not running on Windows"}
}
