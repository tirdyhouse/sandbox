// Example: basic usage of the sandbox package.
package main

import (
	"fmt"
	"log"

	"github.com/steven/sandbox"
)

func main() {
	// Check what's available.
	probe := sandbox.Probe()
	fmt.Printf("Platform:  %s\n", probe.Platform)
	fmt.Printf("Backend:   %s\n", probe.Backend)
	fmt.Printf("Sandboxed: %v\n", probe.Sandboxed)
	if probe.Warning != "" {
		fmt.Printf("Warning:   %s\n", probe.Warning)
	}
	fmt.Println()

	// Run a command with default policy (working directory writable).
	fmt.Println("=== Default policy ===")
	out, err := sandbox.Command("go", "version").Output()
	if err != nil {
		log.Fatalf("command failed: %v", err)
	}
	fmt.Printf("Output: %s", out)

	// Run with custom writable directories.
	fmt.Println("\n=== Custom writable directories ===")
	cmd := sandbox.Command("bash", "-c", "touch /tmp/sandbox-test.txt && echo ok")
	cmd.Policy.WritableDirs = []string{"/tmp"}
	cmd.Dir = "/"
	out, err = cmd.Output()
	if err != nil {
		log.Fatalf("command failed: %v", err)
	}
	fmt.Printf("Output: %s", out)

	// Deny network access.
	fmt.Println("\n=== Network denied ===")
	cmd2 := sandbox.Command("curl", "--max-time", "2", "https://example.com")
	cmd2.Policy.NetworkAccess = sandbox.NetworkDeny
	err = cmd2.Run()
	if err != nil {
		fmt.Printf("Expected error (network blocked): %v\n", err)
	}
}
