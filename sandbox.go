// Package sandbox provides os/exec-compatible command execution with
// filesystem sandboxing — no Docker, no daemon, no extra installation.
//
// On supported platforms (macOS, Linux), sandboxed commands can only write
// to explicitly allowed directories. All other paths are read-only.
// Unsupported platforms fall back to unsafe direct execution with a warning.
//
// Basic usage:
//
//	// Default: working directory is writable, everything else is read-only.
//	out, err := sandbox.Command("go", "test", "./...").Output()
//
//	// Custom writable directories:
//	cmd := sandbox.Command("bash", "-c", "go build -o /workspace/output ./...")
//	cmd.Policy.WritableDirs = []string{"/workspace"}
//	err := cmd.Run()
//
// Platforms:
//
//	macOS   — uses sandbox-exec(1), system built-in, no install needed
//	Linux   — uses Landlock (Linux 5.13+), kernel built-in, no install needed
//	Windows — unsupported (runs unsandboxed with a warning)
package sandbox

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
)

// Cmd represents an external command being prepared or run.
// It mirrors os/exec.Cmd but adds per-command sandbox policies.
type Cmd struct {
	// Path is the path of the command to run.
	Path string

	// Args holds command line arguments, including the command name as Args[0].
	Args []string

	// Env specifies the environment of the process.
	// If nil, the current process's environment is used.
	Env []string

	// Dir specifies the working directory of the command.
	// If empty, runs in the calling process's current directory.
	Dir string

	// Stdin specifies the process's standard input.
	Stdin io.Reader

	// Stdout and Stderr specify the process's standard output and error.
	Stdout io.Writer
	Stderr io.Writer

	// Policy controls sandbox restrictions.
	Policy Policy

	// ProcessState is populated after Wait/Output/CombinedOutput.
	ProcessState *os.ProcessState

	// cmd is the underlying exec.Cmd, set by build().
	cmd *exec.Cmd

	// cleanup holds resources to release after command completion.
	cleanup []func()
}

// Policy defines filesystem restrictions for a sandboxed command.
// Zero value uses sensible defaults (working dir writable).
type Policy struct {
	// WritableDirs lists paths the command is allowed to modify.
	//   nil     → only Dir (or cwd) is writable
	//   empty   → nothing is writable
	//   [paths] → only the listed paths are writable
	WritableDirs []string
}

// Command returns a Cmd to execute the named program with the given arguments.
// The returned Cmd uses default policies (working dir writable).
func Command(name string, arg ...string) *Cmd {
	return &Cmd{
		Path: name,
		Args: append([]string{name}, arg...),
	}
}

// Run starts the command and waits for it to complete.
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Start starts the command but does not wait for it to complete.
func (c *Cmd) Start() error {
	cmd, err := c.build()
	if err != nil {
		return err
	}
	c.cmd = cmd
	return cmd.Start()
}

// Wait waits for the command to exit and releases any sandbox resources.
func (c *Cmd) Wait() error {
	if c.cmd == nil {
		return errors.New("sandbox: command not started")
	}
	err := c.cmd.Wait()
	c.ProcessState = c.cmd.ProcessState
	c.runCleanup()
	return err
}

// Output runs the command and returns its standard output.
func (c *Cmd) Output() ([]byte, error) {
	var buf bytes.Buffer
	if c.Stdout != nil {
		c.Stdout = io.MultiWriter(c.Stdout, &buf)
	} else {
		c.Stdout = &buf
	}
	err := c.Run()
	return buf.Bytes(), err
}

// CombinedOutput runs the command and returns its combined
// standard output and standard error.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	var buf bytes.Buffer
	if c.Stdout != nil {
		c.Stdout = io.MultiWriter(c.Stdout, &buf)
	} else {
		c.Stdout = &buf
	}
	if c.Stderr != nil {
		c.Stderr = io.MultiWriter(c.Stderr, &buf)
	} else {
		c.Stderr = &buf
	}
	err := c.Run()
	return buf.Bytes(), err
}

// build constructs the underlying exec.Cmd, applying sandbox constraints.
func (c *Cmd) build() (*exec.Cmd, error) {
	// Resolve writable dirs: nil defaults to Dir (or cwd).
	writable := c.Policy.WritableDirs
	if writable == nil {
		dir := c.Dir
		if dir == "" {
			dir = "."
		}
		writable = []string{dir}
	}

	// Set default environment.
	env := c.Env
	if env == nil {
		env = os.Environ()
	}

	cmd := exec.Command(c.Path, c.Args[1:]...)
	cmd.Args = c.Args
	cmd.Dir = c.Dir
	cmd.Env = env
	cmd.Stdin = c.Stdin
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr

	// Apply platform-specific sandbox restrictions.
	sb := &sandboxCtx{writable: writable}
	if err := applySandbox(cmd, sb); err != nil {
		return nil, &exec.Error{Name: c.Path, Err: err}
	}
	for _, fn := range sb.cleanup {
		c.addCleanup(fn)
	}

	return cmd, nil
}

// sandboxCtx carries information from the Cmd to platform-specific apply functions.
type sandboxCtx struct {
	writable []string
	cleanup  []func()
}

func (s *sandboxCtx) addCleanup(fn func()) {
	s.cleanup = append(s.cleanup, fn)
}

// addCleanup registers a function to run after command completion.
func (c *Cmd) addCleanup(fn func()) {
	c.cleanup = append(c.cleanup, fn)
}

// runCleanup executes all registered cleanup functions.
func (c *Cmd) runCleanup() {
	for i := len(c.cleanup) - 1; i >= 0; i-- {
		c.cleanup[i]()
	}
	c.cleanup = nil
}

// Available reports whether the current platform supports sandboxing.
// If this returns false, ReasonUnavailable explains why.
func Available() bool { return available() }

// ReasonUnavailable returns why sandboxing is not available on this platform.
// Returns an empty string if sandboxing is available.
func ReasonUnavailable() string { return reasonUnavailable() }

// ProbeResult describes sandbox capabilities on the current platform.
type ProbeResult struct {
	// Sandboxed reports whether sandbox enforcement will be active.
	Sandboxed bool

	// Platform is the detected operating system.
	Platform string

	// Backend is the sandbox technology being used.
	Backend string

	// Warning is a human-readable message about limitations.
	Warning string
}

// Probe detects sandbox capabilities on the current platform.
func Probe() ProbeResult {
	switch runtime.GOOS {
	case "linux":
		return probeLinux()
	case "darwin":
		return probeDarwin()
	case "windows":
		return probeWindows()
	default:
		return ProbeResult{
			Platform: runtime.GOOS,
			Backend:  "none",
			Warning:  "sandboxing not supported on " + runtime.GOOS + "; commands run unsandboxed",
		}
	}
}
