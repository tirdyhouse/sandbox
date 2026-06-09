package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const helperEnv = "SANDBOX_TEST_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "write-files" {
		runWriteFilesHelper()
	}
	os.Exit(m.Run())
}

func TestCommandInitializesExecLikeFields(t *testing.T) {
	cmd := Command("echo", "hello")

	if cmd.Path != "echo" {
		t.Fatalf("Path = %q, want echo", cmd.Path)
	}
	wantArgs := []string{"echo", "hello"}
	if fmt.Sprint(cmd.Args) != fmt.Sprint(wantArgs) {
		t.Fatalf("Args = %v, want %v", cmd.Args, wantArgs)
	}
	if cmd.Policy.WritableDirs != nil {
		t.Fatalf("WritableDirs = %v, want nil default", cmd.Policy.WritableDirs)
	}
}

func TestBuildDefaultsWritableDirToCommandDir(t *testing.T) {
	dir := t.TempDir()
	cmd := Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Dir = dir

	execCmd, err := cmd.build()
	t.Cleanup(cmd.runCleanup)
	if err != nil && Available() {
		t.Fatalf("build returned error on available backend: %v", err)
	}
	if !Available() {
		return
	}
	if execCmd.Dir != dir {
		t.Fatalf("exec cmd Dir = %q, want %q", execCmd.Dir, dir)
	}
}

func TestOutputCapturesStdoutAndPreservesExistingWriter(t *testing.T) {
	cmd := Command(os.Args[0], "-test.run=TestHelperProcess", "--", "print", "hello")
	cmd.Env = append(os.Environ(), helperEnv+"=write-files")

	var existing bytes.Buffer
	cmd.Stdout = &existing

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("Output returned error: %v", err)
	}
	if string(out) != "hello" {
		t.Fatalf("Output = %q, want hello", out)
	}
	if existing.String() != "hello" {
		t.Fatalf("existing stdout = %q, want hello", existing.String())
	}
}

func TestDirectoryWriteProtection(t *testing.T) {
	if !Available() {
		t.Skipf("sandbox unavailable on %s: %s", runtime.GOOS, ReasonUnavailable())
	}

	allowed := t.TempDir()
	denied := t.TempDir()
	cmd := Command(os.Args[0], "-test.run=TestHelperProcess", "--", "write-files", allowed, denied)
	cmd.Env = append(os.Environ(), helperEnv+"=write-files")
	cmd.Policy.WritableDirs = []string{allowed}

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected denied write to fail, output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(allowed, "allowed.txt")); err != nil {
		t.Fatalf("allowed file was not created: %v; output: %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(denied, "denied.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied file exists or stat failed unexpectedly: %v; output: %s", err, out)
	}
}

func TestEmptyWritableDirsAllowsNoWrites(t *testing.T) {
	if !Available() {
		t.Skipf("sandbox unavailable on %s: %s", runtime.GOOS, ReasonUnavailable())
	}

	denied := t.TempDir()
	cmd := Command(os.Args[0], "-test.run=TestHelperProcess", "--", "write-one", filepath.Join(denied, "denied.txt"))
	cmd.Env = append(os.Environ(), helperEnv+"=write-files")
	cmd.Policy.WritableDirs = []string{}

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected write to fail, output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(denied, "denied.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied file exists or stat failed unexpectedly: %v; output: %s", err, out)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "write-files" {
		return
	}
	runWriteFilesHelper()
}

func runWriteFilesHelper() {
	args := os.Args
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep+1 >= len(args) {
		fmt.Fprintln(os.Stderr, "missing helper command")
		os.Exit(2)
	}

	switch args[sep+1] {
	case "print":
		if sep+2 < len(args) {
			fmt.Print(args[sep+2])
		}
	case "write-files":
		if sep+3 >= len(args) {
			fmt.Fprintln(os.Stderr, "missing write-files paths")
			os.Exit(2)
		}
		allowedPath := filepath.Join(args[sep+2], "allowed.txt")
		deniedPath := filepath.Join(args[sep+3], "denied.txt")
		if err := os.WriteFile(allowedPath, []byte("ok"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "allowed write failed: %v\n", err)
			os.Exit(3)
		}
		if err := os.WriteFile(deniedPath, []byte("bad"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "denied write failed as expected: %v\n", err)
			os.Exit(4)
		}
		fmt.Fprintln(os.Stderr, "denied write unexpectedly succeeded")
		os.Exit(5)
	case "write-one":
		if sep+2 >= len(args) {
			fmt.Fprintln(os.Stderr, "missing write-one path")
			os.Exit(2)
		}
		if err := os.WriteFile(args[sep+2], []byte("bad"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write failed as expected: %v\n", err)
			os.Exit(4)
		}
		fmt.Fprintln(os.Stderr, "write unexpectedly succeeded")
		os.Exit(5)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper command %q\n", args[sep+1])
		os.Exit(2)
	}

	if strings.HasPrefix(os.Getenv("__SANDBOX_HELPER"), "1") {
		fmt.Fprintln(os.Stderr, "sandbox helper env leaked")
		os.Exit(6)
	}
	os.Exit(0)
}
