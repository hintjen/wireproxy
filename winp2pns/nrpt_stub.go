//go:build !windows
// +build !windows

package winp2pns

// RegisterNamespace is a stub for non-Windows platforms.
func RegisterNamespace(namespace, dnsServer string) (string, error) {
	return "", ErrNotWindows
}

// UnregisterNamespace is a stub for non-Windows platforms.
func UnregisterNamespace(guid string) error {
	return ErrNotWindows
}

// CleanupStaleEntries is a stub for non-Windows platforms.
func CleanupStaleEntries(namespace string) error {
	return ErrNotWindows
}

// FlushDNSCache is a stub for non-Windows platforms.
func FlushDNSCache() error {
	return ErrNotWindows
}

// GetNRPTEntries is a stub for non-Windows platforms.
func GetNRPTEntries() ([]string, error) {
	return nil, ErrNotWindows
}
