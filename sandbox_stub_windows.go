//go:build windows

package sandbox

// probeLinux is a stub for Windows. The real implementation is in sandbox_linux.go.
func probeLinux() ProbeResult {
	return ProbeResult{Platform: "linux", Backend: "none", Warning: "not running on Linux"}
}

// probeDarwin is a stub for Windows. The real implementation is in sandbox_darwin.go.
func probeDarwin() ProbeResult {
	return ProbeResult{Platform: "darwin", Backend: "none", Warning: "not running on macOS"}
}
