//go:build darwin

package sandbox

// probeLinux is a stub for macOS. The real implementation is in sandbox_linux.go.
func probeLinux() ProbeResult {
	return ProbeResult{Platform: "linux", Backend: "none", Warning: "not running on Linux"}
}

// probeWindows is a stub for macOS. The real implementation is in sandbox_windows.go.
func probeWindows() ProbeResult {
	return ProbeResult{Platform: "windows", Backend: "none", Warning: "not running on Windows"}
}
