//go:build !windows
// +build !windows

package winp2pns

import "errors"

// ErrNotAdmin is returned when administrative privileges are required but not present.
var ErrNotAdmin = errors.New("winp2pns: administrative privileges required")

// IsAdmin always returns an error on non-Windows platforms.
func IsAdmin() (bool, error) {
	return false, ErrNotWindows
}

// IsElevated always returns an error on non-Windows platforms.
func IsElevated() (bool, error) {
	return false, ErrNotWindows
}

// RequireAdmin always returns an error on non-Windows platforms.
func RequireAdmin() error {
	return ErrNotWindows
}
