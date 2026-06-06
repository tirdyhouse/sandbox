# sandbox — os/exec-compatible cross-platform sandbox for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/tirdyhouse/sandbox.svg)](https://pkg.go.dev/github.com/tirdyhouse/sandbox)

**sandbox** is a Go package that provides `os/exec`-compatible command execution with filesystem sandboxing — **no Docker, no daemon, no extra installation**. Just import and use.

> 🚀 **Built for the AI Agent era** — when your LLM generates and executes shell commands, `sandbox` prevents it from writing outside your workspace. No changes to your existing `os/exec` code.

---

**sandbox** 是一个 Go 包，提供 `os/exec` 兼容的命令执行接口，并内置文件系统沙箱——**不需要 Docker、不需要后台进程、不需要额外安装**。直接导入即可使用。

> 🚀 **为 AI Agent 时代而生** —— 当 LLM 自动生成并执行 shell 命令时，`sandbox` 能确保它不会写到工作区以外的目录。**无需修改你现有的 `os/exec` 代码。**

## Quick Start / 快速开始

```go
import "github.com/tirdyhouse/sandbox"

// Just like os/exec — but commands can only write to the working directory.
out, _ := sandbox.Command("go", "build", "./...").Output()

// Custom writable directories:
cmd := sandbox.Command("bash", "-c", "go build -o /workspace/output ./...")
cmd.Policy.WritableDirs = []string{"/workspace"}
cmd.Run()

// Python script, same sandbox:
sandbox.Command("python3", "train.py", "--data", "/workspace/data").Run()

// AI-generated commands, safely sandboxed:
sandbox.Command("bash", "-c", aiGeneratedCommand).Run()
```

## Why sandbox? / 为什么需要沙箱？

**English** | In the age of AI coding assistants, your agent will generate and run commands. Without sandboxing, a single stray command or a miswritten file path can cost hours. `sandbox` is your safety net — it enforces **"read everything, write only workspace"** at the OS level.

**中文** | 在 AI 编程助手时代，你的 Agent 会生成并执行各种命令。没有沙箱保护，一个写错路径的操作就可能造成严重损失。`sandbox` 是你的安全网——它在操作系统层面强制执行 **"可读所有文件，但只能写工作区"**。

### Use cases / 适用场景

| Scenario | What sandbox protects |
|---|---|
| AI agent executing shell commands | Prevents writing to system/config dirs |
| CI/CD build scripts | Ensures artifacts only go to the build dir |
| Multi-tenant code execution | Isolates file writes per user |
| Python/R scripts from untrusted sources | Limits damage from accidental writes |
| `go test` / `npm install` / `pip install` | Prevents cache pollution outside workspace |

## How it works / 工作原理

Each platform uses its OS's **built-in** sandbox mechanism — nothing to install:

| Platform | Backend | Requirement |
|----------|---------|-------------|
| **macOS** | `sandbox-exec(1)` | Built into macOS, ships with the OS |
| **Linux** | **Landlock** (kernel 5.13+) | Built into the Linux kernel |
| **Windows** | **Low Integrity Level** + Mandatory Labels | Built into Windows since Vista |

| Backend | Approach |
|---------|----------|
| macOS | Wraps command with `sandbox-exec(1)` + Seatbelt profile: deny all writes by default, allow only specified paths |
| Linux | Uses Landlock via raw syscalls + self-exec helper. Allows read+execute everywhere, write only in allowed dirs. No extra packages needed (unlike bubblewrap) |
| Windows | Creates a **Low Integrity** token for the child + sets Low Mandatory Label on writable directories. Low IL processes can read Medium IL files but CANNOT write to them |

### Key design / 设计要点

- **No Docker.** No containers, no daemon, no setup scripts.
- **No bubblewrap dependency on Linux.** Landlock is built into the kernel since 5.13.
- **Low Integrity Level on Windows.** The child runs at Low IL — it can read system files but can only write where explicitly allowed.
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

### Policy / 策略

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

> **Note:** On Linux, `NetworkDeny` requires Landlock ABI 3+ (kernel 6.2+). Older kernels do not support network restrictions via Landlock. `Probe()` reports the ABI version. On Windows, network restriction is not yet implemented.

## Platform notes / 平台说明

### macOS

Uses `sandbox-exec(1)` which is bundled with every macOS installation. The Seatbelt profile denies all file writes by default and re-allows only the paths specified in `Policy.WritableDirs`. Temp profile files are cleaned up after command completion.

### Linux (Landlock)

Landlock is an unprivileged Linux Security Module available since kernel 5.13. No packages to install — it's built into the kernel. Uses raw `syscall(2)` — no CGO required.

The self-exec helper pattern: the parent binary re-invokes itself with `__SANDBOX_HELPER=1`, the child sets up Landlock, then `exec`s the real command.

**ABI compatibility:**
- ABI 1 (5.13+): filesystem sandboxing ✅
- ABI 2 (5.19+): adds `LANDLOCK_ACCESS_FS_REFER` for file reparenting ✅
- ABI 3 (6.2+): experimental network restrictions ⚠️
- The library auto-detects the ABI version and adjusts its access mask.

### Windows

Uses **Low Integrity Level** (Windows Vista+):
- The child runs with a Low Integrity token
- Low IL can READ Medium/High IL files but CANNOT WRITE to them
- Writable dirs get a Low Mandatory Label so the child can write
- Label is NOT reverted after completion (parent stays at Medium IL)

**Directory labeling** uses `SetNamedSecurityInfo` with `LABEL_SECURITY_INFORMATION`. The owner of a file can always set its mandatory label — **no admin required** for user-owned files.

## Safety / 安全说明

- This package provides **filesystem write protection**, not a full security boundary.
- Designed to prevent accidental writes to unintended directories, not to contain malicious code.
- For stronger isolation (multi-tenant, untrusted code), pair with a microVM or container runtime.

## Example / 示例

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

## File structure / 文件结构

```
sandbox/
├── sandbox.go              # Core API (Cmd, Policy, Run/Start/Wait/Output)
├── sandbox_darwin.go       # macOS backend: sandbox-exec
├── sandbox_linux.go        # Linux backend: Landlock (raw syscalls)
├── sandbox_windows.go      # Windows backend: Low Integrity Level
├── helper_linux.go         # Linux self-exec helper
├── sandbox_stubs*.go       # Cross-platform stubs
├── examples/main.go        # Usage example
└── README.md
```

---

## Sponsors

<p align="center">
  <a href="https://helix.iqe.me/">
    <strong>Helix</strong>
  </a><br>
  <em>Open AI Agent Platform</em>
</p>

**sandbox** is developed with support from [Helix](https://helix.iqe.me/) — an open AI agent platform that runs every LLM-generated command in a sandboxed environment. Helix provides managed build environments, EasyGateway tunnels for secure internet access, multi-agent coordination, and workspace-driven workflows.

**sandbox** 由 [Helix](https://helix.iqe.me/) 支持开发。Helix 是一个开放 AI Agent 平台，在沙箱环境中安全运行 LLM 生成的每条命令。提供托管构建环境、EasyGateway 穿透隧道、多 Agent 协作和工作区驱动工作流。

> **Helix** is your AI-powered development platform. We sandbox the commands so you don't have to worry. Try it at [helix.iqe.me](https://helix.iqe.me/).

## License

MIT
