//go:build linux

package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"
)

// Linux sandbox backend uses Landlock (Linux 5.13+), which is built into the
// kernel — no additional dependencies required.
//
// Because Landlock restrictions must be applied in the child process before
// exec'ing the target command, we use the self-exec pattern:
//
//  1. applySandbox modifies the Cmd to point to the same running binary
//     (via os.Executable()), with a special env var __SANDBOX_HELPER=1 and a
//     JSON-encoded config in __SANDBOX_CONFIG.
//  2. The child process starts, Go init() detects the helper env var,
//     sets up Landlock restrictions, and syscall.Exec's the real command.
//  3. From that point on, the real command runs with Landlock enforced.
//
// syscall numbers (stable across architectures):
//
//	 444 — landlock_create_ruleset
//	 445 — landlock_add_rule
//	 446 — landlock_restrict_self
const (
	landlockCreateRuleset  = 444
	landlockAddRule        = 445
	landlockRestrictSelf   = 446
	landlockRulePathBeneath = 1
	landlockCreateVersion   = 1

	// Filesystem access rights (from <linux/landlock.h>).
	landlockAccessExecute    = 1 << 0
	landlockAccessWriteFile  = 1 << 1
	landlockAccessReadFile   = 1 << 2
	landlockAccessReadDir    = 1 << 3
	landlockAccessRemoveDir  = 1 << 4
	landlockAccessRemoveFile = 1 << 5
	landlockAccessMakeChar   = 1 << 6
	landlockAccessMakeDir    = 1 << 7
	landlockAccessMakeReg    = 1 << 8
	landlockAccessMakeSock   = 1 << 9
	landlockAccessMakeSym    = 1 << 12
	landlockAccessRefer      = 1 << 13
	landlockAccessTruncate   = 1 << 14

	// Composite: all write-related rights.
	landlockAccessWrite = landlockAccessWriteFile |
		landlockAccessRemoveFile |
		landlockAccessRemoveDir |
		landlockAccessMakeChar |
		landlockAccessMakeDir |
		landlockAccessMakeReg |
		landlockAccessMakeSock |
		landlockAccessMakeSym |
		landlockAccessTruncate |
		landlockAccessRefer

	// All filesystem rights.
	landlockAccessAll = landlockAccessExecute |
		landlockAccessWriteFile |
		landlockAccessReadFile |
		landlockAccessReadDir |
		landlockAccessRemoveFile |
		landlockAccessRemoveDir |
		landlockAccessMakeChar |
		landlockAccessMakeDir |
		landlockAccessMakeReg |
		landlockAccessMakeSock |
		landlockAccessMakeSym |
		landlockAccessTruncate |
		landlockAccessRefer

	atFDCWD = -100 // same as unix.AT_FDCWD
)

type landlockRulesetAttr struct {
	HandledAccessFS uint64
}

type landlockPathBeneathAttr struct {
	AllowedAccess uint64
	ParentFD      int32
	_             [4]byte // padding
}

// helperConfig is serialised to JSON and passed to the child process.
type helperConfig struct {
	WritableDirs []string      `json:"w"`
	Network      NetworkAccess `json:"n"`
}

func available() bool {
	_, _, err := syscall.Syscall(landlockCreateRuleset, 0, 0, landlockCreateVersion)
	return err == 0 || err == syscall.EINVAL || err == syscall.ENOMSG
}

func reasonUnavailable() string {
	if runtime.GOOS != "linux" {
		return "not Linux"
	}
	_, _, err := syscall.Syscall(landlockCreateRuleset, 0, 0, landlockCreateVersion)
	if err == syscall.ENOSYS {
		return "Landlock not supported (kernel < 5.13 or CONFIG_SECURITY_LANDLOCK=n)"
	}
	if err == syscall.EOPNOTSUPP {
		return "Landlock not enabled; add landlock=1 to kernel cmdline"
	}
	if err != 0 && err != syscall.EINVAL && err != syscall.ENOMSG {
		return "Landlock error: " + err.Error()
	}
	return ""
}

func landlockGetABI() int {
	// Calling landlock_create_ruleset with the version flag returns the ABI
	// version as the return value (or a negative errno).
	fd, _, err := syscall.Syscall(landlockCreateRuleset, 0, 0, landlockCreateVersion)
	if err != 0 {
		return 0
	}
	_ = syscall.Close(int(fd))
	return 1
}

func probeLinux() ProbeResult {
	r := ProbeResult{Platform: "linux"}
	abi := landlockGetABI()
	if abi == 0 {
		r.Warning = "Landlock not available"
		r.Backend = "none"
		return r
	}
	r.Backend = fmt.Sprintf("landlock-abi%d", abi)
	if abi >= 1 {
		r.Sandboxed = true
	} else {
		r.Warning = fmt.Sprintf("Landlock ABI %d too old (needs 1+)", abi)
		r.Backend = "none"
	}
	return r
}

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	cfg := helperConfig{
		WritableDirs: ctx.writable,
		Network:      ctx.network,
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("sandbox: marshal config: %w", err)
	}

	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("sandbox: cannot find self path: %w", err)
	}

	// Save original command info.
	origPath := cmd.Path
	origArgs := cmd.Args

	// Rewrite to run via self-exec helper.
	cmd.Path = selfPath
	newArgs := make([]string, 0, len(origArgs)+3)
	newArgs = append(newArgs, selfPath, "__sandbox__", "--", origPath)
	newArgs = append(newArgs, origArgs[1:]...)
	cmd.Args = newArgs
	cmd.Env = append(cmd.Env,
		"__SANDBOX_HELPER=1",
		"__SANDBOX_CONFIG="+string(cfgJSON),
	)

	return nil
}

// setupLandlock applies Landlock rules in the current (child) process.
// Called by the helper init() before exec.
func setupLandlock(cfg *helperConfig) error {
	// Create ruleset.
	attr := landlockRulesetAttr{HandledAccessFS: landlockAccessAll}
	rulesetFd, _, err := syscall.Syscall(landlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		uintptr(unsafe.Sizeof(attr)),
		0,
	)
	if err != 0 {
		return fmt.Errorf("create ruleset: %w", err)
	}
	defer syscall.Close(int(rulesetFd))

	// Allow read+execute on the entire filesystem (block all writes).
	if err := addPathRule(int(rulesetFd), &landlockPathBeneathAttr{
		AllowedAccess: uint64(landlockAccessAll &^ landlockAccessWrite),
	}, "/"); err != nil {
		return fmt.Errorf("allow-read /: %w", err)
	}

	// Allow full access (reads + writes) on specified directories.
	for _, dir := range cfg.WritableDirs {
		if err := addPathRule(int(rulesetFd), &landlockPathBeneathAttr{
			AllowedAccess: uint64(landlockAccessAll),
		}, dir); err != nil {
			return fmt.Errorf("allow-write %q: %w", dir, err)
		}
	}

	// Apply the ruleset to the current process.
	// This is inherited by all future children (but our only child is
	// the real command we're about to exec).
	ret, _, err := syscall.Syscall(landlockRestrictSelf, uintptr(rulesetFd), 0, 0)
	if ret != 0 {
		return fmt.Errorf("restrict self: %w", err)
	}

	return nil
}

// addPathRule adds a path_beneath rule to a Landlock ruleset.
// It opens the directory, adds the rule, and closes the fd.
func addPathRule(rulesetFd int, ruleAttr *landlockPathBeneathAttr, dir string) error {
	dirFd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %q: %w", dir, err)
	}
	defer syscall.Close(dirFd)

	ruleAttr.ParentFD = int32(dirFd)
	ret, _, errno := syscall.Syscall(landlockAddRule,
		uintptr(rulesetFd),
		uintptr(landlockRulePathBeneath),
		uintptr(unsafe.Pointer(ruleAttr)),
	)
	if ret != 0 {
		return errno
	}
	return nil
}
