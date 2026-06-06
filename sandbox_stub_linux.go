//go:build linux

package sandbox

// probeDarwin is a stub for Linux. The real implementation is in sandbox_darwin.go.
func probeDarwin() ProbeResult {
	return ProbeResult{Platform: "darwin", Backend: "none", Warning: "not running on macOS"}
}
