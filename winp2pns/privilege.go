//go:build windows
// +build windows

package winp2pns

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// ErrNotAdmin is returned when administrative privileges are required but not present.
var ErrNotAdmin = errors.New("winp2pns: administrative privileges required")

// IsAdmin checks if the current process has administrative privileges.
// This is required for:
//   - Binding to port 53 (privileged port)
//   - Modifying NRPT registry keys in HKLM
func IsAdmin() (bool, error) {
	var sid *windows.SID

	// Create a SID for the Administrators group
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false, fmt.Errorf("failed to create admin SID: %w", err)
	}
	defer windows.FreeSid(sid)

	// Check if the current token is a member of the Administrators group
	token := windows.Token(0) // Current process token
	member, err := token.IsMember(sid)
	if err != nil {
		return false, fmt.Errorf("failed to check admin membership: %w", err)
	}

	return member, nil
}

// IsElevated checks if the current process token is elevated.
// On Windows Vista+, even admin accounts run with a filtered token.
// This checks if the process was started with "Run as Administrator".
func IsElevated() (bool, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false, fmt.Errorf("failed to open process token: %w", err)
	}
	defer token.Close()

	return token.IsElevated(), nil
}

// RequireAdmin returns an error if the current process is not running
// with elevated administrative privileges.
func RequireAdmin() error {
	elevated, err := IsElevated()
	if err != nil {
		return fmt.Errorf("failed to check elevation status: %w", err)
	}

	if !elevated {
		return ErrNotAdmin
	}

	return nil
}
