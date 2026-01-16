//go:build windows
// +build windows

package winp2pns

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/windows/registry"
)

const (
	// nrptBasePath is the registry path for NRPT configuration
	nrptBasePath = `SOFTWARE\Policies\Microsoft\Windows NT\DNSClient\DnsPolicyConfig`
)

// RegisterNamespace creates an NRPT rule to redirect queries for the given
// namespace suffix to the local DNS server at 127.0.0.1.
//
// For example, with namespace ".p2p.local", all DNS queries ending with
// ".p2p.local" will be sent to 127.0.0.1 instead of the system DNS.
//
// Returns the GUID of the created registry entry (used for cleanup).
func RegisterNamespace(namespace, dnsServer string) (string, error) {
	// Generate a unique GUID for this NRPT entry
	guid := "{" + strings.ToUpper(uuid.New().String()) + "}"

	// First, clean up any stale entries from previous crashes
	if err := CleanupStaleEntries(namespace); err != nil {
		// Log but don't fail - we might not have permission yet
		_ = err
	}

	// Open or create the DnsPolicyConfig key
	key, _, err := registry.CreateKey(
		registry.LOCAL_MACHINE,
		nrptBasePath+`\`+guid,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create NRPT registry key: %w", err)
	}
	defer key.Close()

	// Set the namespace suffix (Name) - REG_MULTI_SZ
	// The namespace should include the leading dot
	if err := key.SetStringsValue("Name", []string{namespace}); err != nil {
		return "", fmt.Errorf("failed to set Name value: %w", err)
	}

	// Set the DNS server to use (GenericDNSServers) - REG_SZ
	if err := key.SetStringValue("GenericDNSServers", dnsServer); err != nil {
		return "", fmt.Errorf("failed to set GenericDNSServers value: %w", err)
	}

	// Enable the rule (ConfigOptions) - REG_DWORD
	if err := key.SetDWordValue("ConfigOptions", 1); err != nil {
		return "", fmt.Errorf("failed to set ConfigOptions value: %w", err)
	}

	return guid, nil
}

// UnregisterNamespace removes the NRPT rule identified by the given GUID.
func UnregisterNamespace(guid string) error {
	if guid == "" {
		return nil
	}

	err := registry.DeleteKey(registry.LOCAL_MACHINE, nrptBasePath+`\`+guid)
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("failed to delete NRPT registry key: %w", err)
	}

	return nil
}

// CleanupStaleEntries removes any orphaned NRPT entries that match the given
// namespace. This handles cases where a previous instance crashed without
// cleaning up its registry entries.
func CleanupStaleEntries(namespace string) error {
	// Open the DnsPolicyConfig key
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		nrptBasePath,
		registry.READ|registry.WRITE,
	)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil // No entries to clean
		}
		return fmt.Errorf("failed to open NRPT registry key: %w", err)
	}
	defer key.Close()

	// Enumerate all subkeys (each is a GUID)
	subkeys, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return fmt.Errorf("failed to enumerate NRPT subkeys: %w", err)
	}

	for _, subkey := range subkeys {
		// Open the subkey to check its namespace
		subkeyPath := nrptBasePath + `\` + subkey
		sk, err := registry.OpenKey(registry.LOCAL_MACHINE, subkeyPath, registry.READ)
		if err != nil {
			continue
		}

		// Check if this entry is for our namespace
		names, _, err := sk.GetStringsValue("Name")
		sk.Close()
		if err != nil {
			continue
		}

		for _, name := range names {
			if name == namespace {
				// This is a stale entry for our namespace - remove it
				_ = registry.DeleteKey(registry.LOCAL_MACHINE, subkeyPath)
				break
			}
		}
	}

	return nil
}

// FlushDNSCache forces Windows to clear its DNS resolver cache.
// This ensures that the new NRPT rules take effect immediately.
func FlushDNSCache() error {
	cmd := exec.Command("ipconfig", "/flushdns")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to flush DNS cache: %w", err)
	}
	return nil
}

// GetNRPTEntries returns a list of all NRPT entries for debugging purposes.
func GetNRPTEntries() ([]string, error) {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		nrptBasePath,
		registry.READ,
	)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open NRPT registry key: %w", err)
	}
	defer key.Close()

	return key.ReadSubKeyNames(-1)
}
