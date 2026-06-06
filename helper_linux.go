//go:build linux

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// init detects sandbox helper mode at the earliest possible point.
//
// When a command is sandboxed via Landlock, applySandbox rewrites the Cmd
// to point at this same binary with __SANDBOX_HELPER=1. The child process
// enters this init(), sets up Landlock, and exec's the real command.
// This function never returns in the helper case.
func init() {
	if os.Getenv("__SANDBOX_HELPER") != "1" {
		return
	}

	// Args layout: <self> __sandbox__ -- <realCmd> [args...]
	args := os.Args
	sepIdx := -1
	for i, a := range args {
		if a == "--" {
			sepIdx = i
			break
		}
	}
	if sepIdx < 0 || sepIdx+1 >= len(args) {
		fmt.Fprintf(os.Stderr, "sandbox: malformed helper args: %v\n", args)
		os.Exit(1)
	}

	realPath := args[sepIdx+1]
	realArgs := args[sepIdx+1:]

	// Decode sandbox config from environment.
	var cfg helperConfig
	cfgJSON := os.Getenv("__SANDBOX_CONFIG")
	if cfgJSON == "" {
		fmt.Fprintln(os.Stderr, "sandbox: missing __SANDBOX_CONFIG")
		os.Exit(1)
	}
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: invalid config: %v\n", err)
		os.Exit(1)
	}

	// Apply Landlock restrictions.
	if err := setupLandlock(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: landlock setup failed: %v\n", err)
		os.Exit(1)
	}

	// Resolve the real command if not absolute.
	resolvedPath := realPath
	if !filepath.IsAbs(resolvedPath) {
		if found, err := exec.LookPath(resolvedPath); err == nil {
			resolvedPath = found
		}
	}

	// Strip sandbox helper variables from environment.
	cleanEnv := os.Environ()
	filtered := cleanEnv[:0]
	for _, e := range cleanEnv {
		if strings.HasPrefix(e, "__SANDBOX_") {
			continue
		}
		filtered = append(filtered, e)
	}

	// Replace current process with the real command.
	// syscall.Exec never returns on success.
	if err := syscall.Exec(resolvedPath, realArgs, filtered); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: exec %s: %v\n", resolvedPath, err)
		os.Exit(1)
	}
}
