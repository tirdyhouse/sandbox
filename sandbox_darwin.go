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
	_, err := exec.LookPath("sandbox-exec")
	return err == nil
}

func reasonUnavailable() string {
	if runtime.GOOS != "darwin" {
		return "not macOS"
	}
	_, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return "sandbox-exec not found (should be at /usr/bin/sandbox-exec)"
	}
	return ""
}

func probeDarwin() ProbeResult {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return ProbeResult{
			Platform: "darwin",
			Backend:  "none",
			Warning:  "sandbox-exec not found: " + err.Error(),
		}
	}
	return ProbeResult{
		Sandboxed: true,
		Platform:  "darwin",
		Backend:   "sandbox-exec",
	}
}

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	// Generate sandbox profile.
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

	// Register cleanup for the temp profile file.
	ctx.addCleanup(func() { os.Remove(profilePath) })

	// Restructure the command to run via sandbox-exec.
	// From:   mycmd arg1 arg2
	// To:     sandbox-exec -f profile.sb mycmd arg1 arg2
	sandboxExecPath, err := exec.LookPath("sandbox-exec")
	if err != nil {
		return fmt.Errorf("sandbox: sandbox-exec not found: %w", err)
	}

	origPath := cmd.Path
	origArgs := cmd.Args

	cmd.Path = sandboxExecPath
	cmd.Args = append([]string{
		"sandbox-exec",
		"-f", profilePath,
		"--",
		origPath,
	}, origArgs[1:]...)

	return nil
}
