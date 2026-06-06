//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// macOS sandbox backend uses the built-in sandbox-exec(1) command.
// No additional dependencies required — it ships with every macOS install.

var _sandboxExecPath string // cached by init()

func init() {
	_sandboxExecPath, _ = exec.LookPath("sandbox-exec")
}

const macOSDefaultPolicy = `(version 1)

(allow default)

; Deny all file writes by default.
(deny file-write*)

; Allow writes to explicit directories.
%s

; Allow reading all files.
(allow file-read*)

; Allow process execution.
(allow process*)

; Allow sysctl read (uname, etc.).
(allow sysctl-read)

; Network policy.
%s
`

func available() bool {
	return _sandboxExecPath != ""
}

func reasonUnavailable() string {
	if runtime.GOOS != "darwin" {
		return "not macOS"
	}
	if _sandboxExecPath == "" {
		return "sandbox-exec not found (should be at /usr/bin/sandbox-exec)"
	}
	return ""
}

func probeDarwin() ProbeResult {
	if _sandboxExecPath == "" {
		return ProbeResult{
			Platform: "darwin",
			Backend:  "none",
			Warning:  "sandbox-exec not found",
		}
	}
	return ProbeResult{
		Sandboxed: true,
		Platform:  "darwin",
		Backend:   "sandbox-exec",
	}
}

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	// Generate sandbox profile content.
	allowWrites := new(strings.Builder)
	for _, p := range ctx.writable {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("sandbox: resolve writable path %q: %w", p, err)
		}
		fmt.Fprintf(allowWrites, "(allow file-write* (subpath %q))\n", abs)
	}

	var netPolicy string
	switch ctx.network {
	case NetworkDeny:
		netPolicy = "(deny network*)\n(allow network* (only localhost))"
	default:
		netPolicy = "(allow network*)"
	}

	profile := fmt.Sprintf(macOSDefaultPolicy, allowWrites.String(), netPolicy)

	// Write profile to a temp file.
	f, err := os.CreateTemp("", "sandbox-*.sb")
	if err != nil {
		return fmt.Errorf("sandbox: create profile: %w", err)
	}
	profilePath := f.Name()
	if _, err := f.WriteString(profile); err != nil {
		f.Close()
		os.Remove(profilePath)
		return fmt.Errorf("sandbox: write profile: %w", err)
	}
	f.Close()

	// Register cleanup for the temp profile file on successful command completion.
	// If we error out before returning, we clean up manually.
	cleanupOK := false
	defer func() {
		if !cleanupOK {
			os.Remove(profilePath)
		}
	}()

	// Build the sandbox-exec command.
	origPath := cmd.Path
	origArgs := cmd.Args

	cmd.Path = _sandboxExecPath
	cmd.Args = append([]string{
		"sandbox-exec",
		"-f", profilePath,
		"--",
		origPath,
	}, origArgs[1:]...)

	cleanupOK = true
	ctx.addCleanup(func() { os.Remove(profilePath) })
	return nil
}
