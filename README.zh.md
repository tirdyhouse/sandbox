# sandbox — 兼容 os/exec 的 Go 目录写保护沙箱

[![Go Reference](https://pkg.go.dev/badge/github.com/tirdyhouse/sandbox.svg)](https://pkg.go.dev/github.com/tirdyhouse/sandbox)

[English](README.md) | [中文](README.zh.md)

**sandbox** 是一个 Go 包，提供 `os/exec` 兼容的命令执行接口，并内置目录写保护——**不需要 Docker、不需要后台进程、不需要额外安装**。直接导入即可使用。

> 🚀 **为 AI Agent 时代而生** —— 当 LLM 自动生成并执行 shell 命令时，`sandbox` 能确保它不会写到工作区以外的目录。**无需修改你现有的 `os/exec` 代码。**

## 快速开始

```go
import "github.com/tirdyhouse/sandbox"

// 和 os/exec 用法一样——但命令只能写工作目录。
out, _ := sandbox.Command("go", "build", "./...").Output()

// 自定义可写目录：
cmd := sandbox.Command("bash", "-c", "go build -o /workspace/output ./...")
cmd.Policy.WritableDirs = []string{"/workspace"}
cmd.Run()

// 跑 Python 脚本，同样受目录写保护：
sandbox.Command("python3", "train.py").Run()

// AI 生成的命令，限制在可写目录内：
sandbox.Command("bash", "-c", aiGeneratedCommand).Run()
```

## 为什么需要沙箱？

在 AI 编程助手时代，你的 Agent 会生成并执行各种命令。没有沙箱保护，一个写错路径的操作就可能造成严重损失。`sandbox` 是你的安全网——它在操作系统层面强制执行 **"可读所有文件，但只能写工作区"**。

### 适用场景

| 场景 | 沙箱保护什么 |
|---|---|
| AI Agent 执行 shell 命令 | 防止写到系统/配置目录 |
| CI/CD 构建脚本 | 确保产物只输出到构建目录 |
| 本地构建/测试自动化 | 把生成文件限制在工作区内 |
| AI 生成的 Python/R 脚本 | 限制误写造成的损失 |
| `go test` / `npm install` / `pip install` | 防止缓存污染工作区外目录 |

## 工作原理

每个平台使用操作系统**内置**的沙箱机制——无需额外安装：

| 平台 | 后端 | 要求 |
|----------|---------|-------------|
| **macOS** | `sandbox-exec(1)` | macOS 自带 |
| **Linux** | **Landlock**（内核 5.13+）| Linux 内核内置 |
| **Windows** | **Low Integrity Level** + 强制标签 | Windows Vista 起内置 |

### 后端详解

**macOS** — 用 `sandbox-exec(1)` 包装命令 + Seatbelt 策略文件：默认拒绝所有文件写入，只允许指定路径。临时策略文件在命令完成后自动清理。

**Linux** — 使用 **Landlock**（Linux 安全模块），通过 raw syscall 实现（无 CGO）。采用 self-exec helper 模式：父进程重新调用自己，子进程设置 Landlock 后 `exec` 真实命令。**不需要 bubblewrap 或其他额外安装包。** ABI 自动检测确保跨内核版本兼容（ABI 1+ / Linux 5.13+）。

**Windows** — 为子进程创建 **Low Integrity** token + 通过 `SetNamedSecurityInfo` 在可写目录上设置 Low 强制标签。Low IL 进程能读 Medium/High IL 文件，但**不能写**。用户自己的目录不需要管理员权限。

### 设计要点

- **只做目录写保护。** 本包有意不拦截网络访问。
- **不需要 Docker。** 没有容器、没有后台进程、没有配置脚本。
- **Linux 不需要 bubblewrap。** Landlock 从内核 5.13 起内置。
- **Windows 使用 Low Integrity Level。** 可广泛读取，只能在允许目录写入。
- **API 兼容 os/exec。** 可直接替换你现有的命令执行代码。
- **每个命令独立策略。** 每个 `Cmd` 有自己的 `Policy.WritableDirs`。

## API

```go
// 创建沙箱命令——和 os/exec.Command 签名一致。
sandbox.Command(name string, arg ...string) *Cmd

// 执行方法——与 os/exec 完全相同。
(*Cmd).Run()                 // 同步执行
(*Cmd).Start() / .Wait()     // 异步执行
(*Cmd).Output()              // 捕获 stdout
(*Cmd).CombinedOutput()      // 捕获 stdout+stderr

// 能力检测。
sandbox.Available()          // 当前平台是否支持目录写保护？
sandbox.Probe() ProbeResult  // 详细能力信息
sandbox.ReasonUnavailable()  // 为什么不支持
```

### 策略配置

```go
cmd := sandbox.Command("python", "script.py")

// WritableDirs:
//   nil     → 只有 Dir（或当前目录）可写（默认）
//   empty   → 所有目录都不可写
//   [paths] → 只有列出的路径可写
cmd.Policy.WritableDirs = []string{"/workspace"}
```

## 安全说明

- 本包提供**文件系统写保护**，不是完整的安全边界。
- 本包**不**拦截网络访问、进程执行、CPU/内存使用，也不阻止读取操作系统可见文件。
- 设计用于防止**意外**写入非预期目录，不能用于隔离恶意代码。
- 如需更强隔离（多租户、不可信代码），建议搭配微 VM 或容器运行时。

## 示例

```go
package main

import (
	"fmt"
	"log"

	"github.com/tirdyhouse/sandbox"
)

func main() {
	probe := sandbox.Probe()
	fmt.Printf("后端: %s | 沙箱: %v\n", probe.Backend, probe.Sandboxed)

	out, err := sandbox.Command("go", "version").Output()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(out))
}
```

## 文件结构

```
sandbox/
├── sandbox.go              # 核心 API（Cmd、Policy、Run/Start/Wait/Output）
├── sandbox_darwin.go       # macOS 后端：sandbox-exec
├── sandbox_linux.go        # Linux 后端：Landlock（raw syscall）
├── sandbox_windows.go      # Windows 后端：Low Integrity Level
├── helper_linux.go         # Linux self-exec helper
├── sandbox_stubs*.go       # 跨平台编译桩
├── examples/main.go        # 使用示例
├── README.md               # English docs
├── README.zh.md            # 本文件
└── go.mod / go.sum
```

---

## 赞助

<p align="center">
  <a href="https://helix.iqe.me/"><strong>Helix</strong></a> — <em>开放 AI Agent 平台</em>
</p>

**sandbox** 由 [Helix](https://helix.iqe.me/) 支持开发。Helix 是一个开放 AI Agent 平台，会把 LLM 生成命令的写入限制在预期工作区内。Helix 坚持**全工具链纯 Go 实现**——从 Agent 运行时、LLM 路由、沙箱、MCP 工具层到代码分析，全部用 Go 编写，零外部运行时依赖。

Helix 提供托管构建环境、[EasyGateway](https://helix.iqe.me/) 穿透隧道、多 Agent 协作和工作区驱动工作流。**我们限制命令写入范围，让你不用担心误写目录。**

> 立即体验：[helix.iqe.me](https://helix.iqe.me/)

## 开源协议

MIT
