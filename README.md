# sandbox — `os/exec`-style sandbox for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/tirdyhouse/sandbox.svg)](https://pkg.go.dev/github.com/steven/sandbox)
[![Go Report Card](https://goreportcard.com/badge/github.com/tirdyhouse/sandbox)](https://goreportcard.com/report/github.com/steven/sandbox)

**sandbox** is a Go package that provides `os/exec`-compatible command execution with filesystem sandboxing — **no Docker, no daemon, no extra installation**.

```go
import "github.com/tirdyhouse/sandbox"

// Default: working directory is writable, everything else is read-only.
out, _ := sandbox.Command("go", "test", "./...").Output()

// Custom writable directories:
cmd := sandbox.Command("bash", "-c", "go build -o /workspace/output ./...")
cmd.Policy.WritableDirs = []string{"/workspace"}
cmd.Run()

// Block network access:
cmd2 := sandbox.Command("curl", "https://example.com")
cmd2.Policy.NetworkAccess = sandbox.NetworkDeny
cmd2.Run()
```

## How it works

Each platform uses its OS's built-in sandbox mechanism — nothing to install:

| Platform | Backend | Requirement |
|----------|---------|-------------|
| **macOS** | `sandbox-exec(1)` | Built into macOS, ships with the OS |
| **Linux** | Landlock (LSM) | Linux kernel 5.13+, built-in |
| **Windows** | — | Not supported (runs unsandboxed with a warning) |

### macOS

The command is wrapped with `sandbox-exec(1)` using a dynamically generated Seatbelt profile. The profile denies all file writes by default and re-allows only the paths specified in `Policy.WritableDirs`.

### Linux

Uses Landlock (Linux Security Module, kernel 5.13+) via the `golang.org/x/sys/unix` package. Because Landlock must be applied in the child process before the target command runs, `sandbox` uses a **self-exec pattern**: the binary re-invokes itself with a special environment variable, sets up Landlock rules, then `exec`s the real command.

Landlock provides unprivileged filesystem sandboxing — no root, no containers, no daemon.

### Other platforms

Sandboxing is unavailable. Commands run unsandboxed with a warning. Check `sandbox.Available()` or `sandbox.Probe()` to determine if enforcement is active.

## API

The API mirrors `os/exec`:

| Function / Method | Description |
|-------------------|-------------|
| `Command(name, args...)` | Create a sandboxed command |
| `(*Cmd).Run()` | Run and wait |
| `(*Cmd).Start()` / `(*Cmd).Wait()` | Async execution |
| `(*Cmd).Output()` | Run and capture stdout |
| `(*Cmd).CombinedOutput()` | Run and capture stdout+stderr |
| `Available()` | Check if sandboxing is supported |
| `Probe()` | Get detailed platform capability info |
| `ReasonUnavailable()` | Get human-readable reason |

## Policy

```go
cmd := sandbox.Command("bash", "-c", "./build.sh")

// WritableDirs:
//   nil     → only Dir (or cwd) is writable
//   empty   → nothing is writable
//   [paths] → only the listed paths are writable
cmd.Policy.WritableDirs = []string{"/workspace"}

// NetworkAccess:
cmd.Policy.NetworkAccess = sandbox.NetworkAllow  // default
cmd.Policy.NetworkAccess = sandbox.NetworkDeny   // block network
```

## Example

```go
package main

import (
	"fmt"
	"log"
	"github.com/tirdyhouse/sandbox"
)

func main() {
	probe := sandbox.Probe()
	fmt.Printf("Backend: %s (sandboxed: %v)\n", probe.Backend, probe.Sandboxed)

	out, err := sandbox.Command("go", "version").Output()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(out))
}
```

## Safety notes

- macOS: sandbox profile temp files are cleaned up after execution.
- Linux: The parent process's Landlock restrictions are **not** affected — only the child process is restricted via the self-exec helper.
- This package provides **filesystem sandboxing**, not a full security boundary. It's designed to prevent accidental writes to unintended directories, not to contain malicious code.

## License

MIT
