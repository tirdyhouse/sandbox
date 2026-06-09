//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows sandbox backend uses:
//   - Low Integrity Level: prevents writes to Medium/High IL directories
//   - Restricted Token: strips dangerous privileges
//   - Mandatory Labels on writable directories (Low IL allows child writes)
//
// No additional installation required — all APIs are part of Windows since Vista.
//
// Limitations:
//   - Writable directories get a Low Mandatory Label to allow child writes.
//   - The label is NOT reverted after command completion (the parent process
//     at Medium IL can still access them normally).

func available() bool { return true }

func reasonUnavailable() string {
	if runtime.GOOS != "windows" {
		return "not Windows"
	}
	return ""
}

func probeWindows() ProbeResult {
	return ProbeResult{
		Sandboxed: true,
		Platform:  "windows",
		Backend:   "integrity-level",
	}
}

// lowIL is the Low Integrity SID (S-1-16-4096), created once.
var lowIL = func() *windows.SID {
	sid, err := windows.StringToSid("S-1-16-4096")
	if err != nil {
		panic("sandbox: create Low IL SID: " + err.Error())
	}
	return sid
}()

func applySandbox(cmd *exec.Cmd, ctx *sandboxCtx) error {
	// Step 1: Set Low Mandatory Label on writable directories.
	// The child runs at Low IL, so it can only write to Low IL directories.
	if err := setLowLabelOnDirs(ctx.writable); err != nil {
		return fmt.Errorf("sandbox: label directories: %w", err)
	}

	// Step 2: Open the current process token.
	var token windows.Token
	if err := windows.OpenProcessToken(
		windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ASSIGN_PRIMARY,
		&token,
	); err != nil {
		return fmt.Errorf("sandbox: open token: %w", err)
	}
	defer token.Close()

	// Step 3: Duplicate the token to create a modifiable primary token.
	var dupToken windows.Token
	if err := windows.DuplicateTokenEx(
		token,
		windows.TOKEN_ALL_ACCESS,
		nil,                       // security attributes
		windows.SecurityAnonymous, // impersonation level
		windows.TokenPrimary,      // token type
		&dupToken,
	); err != nil {
		return fmt.Errorf("sandbox: duplicate token: %w", err)
	}

	// Step 4: Set Low Integrity Level on the duplicated token.
	if err := setTokenLowIL(dupToken); err != nil {
		dupToken.Close()
		return err
	}

	// Step 5: Assign the restricted token to the child process.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Token = syscall.Token(dupToken)

	return nil
}

// setTokenLowIL sets TokenIntegrityLevel to Low on the given token.
func setTokenLowIL(token windows.Token) error {
	type sidAndAttrs struct {
		Sid        *windows.SID
		Attributes uint32
	}
	type mandatoryLabel struct {
		Label sidAndAttrs
	}

	info := mandatoryLabel{
		Label: sidAndAttrs{
			Sid:        lowIL,
			Attributes: 0x20, // SE_GROUP_INTEGRITY
		},
	}

	return windows.SetTokenInformation(
		token,
		windows.TokenIntegrityLevel,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
}

// setLowLabelOnDirs sets Low Mandatory Label on each writable directory.
func setLowLabelOnDirs(dirs []string) error {
	for _, dir := range dirs {
		abs, err := windows.FullPath(dir)
		if err != nil {
			return fmt.Errorf("resolve %q: %w", dir, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return fmt.Errorf("stat %q: %w", abs, err)
		}
		if err := setLowLabel(abs); err != nil {
			return fmt.Errorf("label %q: %w", abs, err)
		}
	}
	return nil
}

// setLowLabel sets a Low Mandatory Label on a file/directory.
// The owner of an object can always set its mandatory label, so this
// does NOT require administrator privileges for user-owned files.
func setLowLabel(path string) error {
	// Build a SYSTEM_MANDATORY_LABEL_ACE using the EXPLICIT_ACCESS API.
	ea := windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_READ | windows.GENERIC_WRITE | windows.GENERIC_EXECUTE,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       windows.CONTAINER_INHERIT_ACE | windows.OBJECT_INHERIT_ACE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(lowIL),
		},
	}

	// Create an ACL containing this ACE.
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{ea}, nil)
	if err != nil {
		return fmt.Errorf("build acl: %w", err)
	}

	// Set the SACL (LABEL_SECURITY_INFORMATION) on the path.
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.LABEL_SECURITY_INFORMATION,
		nil, // owner
		nil, // group
		nil, // DACL
		acl, // SACL (mandatory label)
	); err != nil {
		return fmt.Errorf("SetNamedSecurityInfo: %w", err)
	}

	return nil
}
