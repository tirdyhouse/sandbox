# sandbox — os/exec-compatible cross-platform sandbox for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/tirdyhouse/sandbox.svg)](https://pkg.go.dev/github.com/tirdyhouse/sandbox)

[中文](README.zh.md) | [English](README.md)

**sandbox** is a Go package that provides `os/exec`-compatible command execution with filesystem sandboxing — **no Docker, no daemon, no extra installation**. Just import and use.

> 🚀 **Built for the AI Agent era** — when your LLM generates and executes shell commands, `sandbox` prevents it from writing outside your workspace. No changes to your existing `os/exec` code.

## Quick Start

```go
import "github.com/tirdyhouse/sandbox"

// Just like os/exec — but commands can only write to the working directory.
out, _ := sandbox.Command("go", "build", "./...").Output()

// Custom writable directories:
cmd := sandbox.Command("bash", "-c", "go build -o /workspace/output ./...")
cmd.Policy.WritableDirs = []string{"/workspace"}
cmd.Run()

// Python script, same sandbox:
sandbox.Command("python3", "train.py").Run()

// AI-generated commands, safely sandboxed:
sandbox.Command("bash", "-c", aiGeneratedCommand).Run()
```

## Why sandbox?

In the age of AI coding assistants, your agent will generate and run commands. Without sandboxing, a single stray command or a miswritten file path can cost hours. `sandbox` is your safety net — it enforces **"read everything, write only workspace"** at the OS level.

### Use cases

| Scenario | What sandbox protects |
|---|---|
| AI agent executing shell commands | Prevents writing to system/config dirs |
| CI/CD build scripts | Ensures artifacts only go to the build dir |
| Multi-tenant code execution | Isolates file writes per user |
| Python/R scripts from untrusted sources | Limits damage from accidental writes |
| `go test` / `npm install` / `pip install` | Prevents cache pollution outside workspace |

## How it works

Each platform uses its OS's **built-in** sandbox mechanism — nothing to install:

| Platform | Backend | Requirement |
|----------|---------|-------------|
| **macOS** | `sandbox-exec(1)` | Built into macOS |
| **Linux** | **Landlock** (kernel 5.13+) | Built into the Linux kernel |
| **Windows** | **Low Integrity Level** + Mandatory Labels | Built into Windows since Vista |

### Backend details

**macOS** — Wraps command with `sandbox-exec(1)` + Seatbelt profile: deny all writes by default, allow only specified paths. Temp profile files cleaned up after command completion.

**Linux** — Uses **Landlock** via raw syscalls (no CGO). The self-exec helper pattern: parent re-invokes itself with helper env, child sets up Landlock, then `exec`s the real command. **No bubblewrap or extra packages needed.** ABI auto-detection ensures compatibility across kernel versions (ABI 1+ / Linux 5.13+).

**Windows** — Creates a **Low Integrity** token for the child + sets Low Mandatory Label on writable directories via `SetNamedSecurityInfo`. Low IL processes can read Medium/High IL files but CANNOT write to them. No admin required for user-owned directories.

### Key design

- **No Docker.** No containers, no daemon, no setup scripts.
- **No bubblewrap dependency on Linux.** Landlock is built into the kernel since 5.13.
- **Low Integrity Level on Windows.** Read everything, write only where allowed.
- **os/exec compatible API.** Drop-in replacement for your existing command execution code.
- **Per-command policy.** Each `Cmd` has its own `Policy.WritableDirs` and `Policy.NetworkAccess`.

## API

```go
// Create a sandboxed command — same signature as os/exec.Command.
sandbox.Command(name string, arg ...string) *Cmd

// Run methods — identical to os/exec.
(*Cmd).Run()                 // Run and wait
(*Cmd).Start() / .Wait()     // Async execution
(*Cmd).Output()              // Capture stdout
(*Cmd).CombinedOutput()      // Capture stdout+stderr

// Capability detection.
sandbox.Available()          // Is sandboxing supported?
sandbox.Probe() ProbeResult  // Detailed capability info
sandbox.ReasonUnavailable()  // Why sandboxing is unavailable
```

### Policy

```go
cmd := sandbox.Command("python", "script.py")

// WritableDirs:
//   nil     → only Dir (or cwd) is writable (default)
//   empty   → nothing is writable
//   [paths] → only the listed paths are writable
cmd.Policy.WritableDirs = []string{"/workspace"}

// NetworkAccess:
cmd.Policy.NetworkAccess = sandbox.NetworkAllow   // default
cmd.Policy.NetworkAccess = sandbox.NetworkDeny    // block network
```

> **Note:** On Linux, `NetworkDeny` requires Landlock ABI 3+ (kernel 6.2+). `Probe()` reports the ABI version. On Windows, network restriction is not yet implemented.

## Safety

- This package provides **filesystem write protection**, not a full security boundary.
- Designed to prevent accidental writes to unintended directories, not to contain malicious code.
- For stronger isolation (multi-tenant, untrusted code), pair with a microVM or container runtime.

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
	fmt.Printf("Backend: %s | Sandboxed: %v\n", probe.Backend, probe.Sandboxed)

	out, err := sandbox.Command("go", "version").Output()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(out))
}
```

## File structure

```
sandbox/
├── sandbox.go              # Core API (Cmd, Policy, Run/Start/Wait/Output)
├── sandbox_darwin.go       # macOS backend: sandbox-exec
├── sandbox_linux.go        # Linux backend: Landlock (raw syscalls)
├── sandbox_windows.go      # Windows backend: Low Integrity Level
├── helper_linux.go         # Linux self-exec helper
├── sandbox_stubs*.go       # Cross-platform stubs
├── examples/main.go        # Usage example
├── README.md               # This file
├── README.zh.md            # 中文文档
└── go.mod / go.sum
```

---

## Sponsors

<p align="center">
  <a href="https://helix.iqe.me/"><strong>Helix</strong></a> — <em>Open AI Agent Platform</em>
</p>

**sandbox** is developed with support from [Helix](https://helix.iqe.me/) — an open AI agent platform that runs every LLM-generated command in a sandboxed environment.

Helix provides managed build environments, [EasyGateway](https://helix.iqe.me/) tunnels for secure internet access, multi-agent coordination, and workspace-driven workflows. **We sandbox the commands, so you don't have to worry.**

> Try Helix at [helix.iqe.me](https://helix.iqe.me/)

## License

MIT
