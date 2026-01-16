// Package winp2pns provides Windows-specific P2P DNS resolution using NRPT.
// It intercepts developer-defined domains (e.g., *.p2p.local) and provides
// DNS SRV records to map friendly domain names to dynamic local proxy ports.
package winp2pns

import (
	"errors"
	"sync"

	"github.com/miekg/dns"
)

// Common errors
var (
	// ErrNotWindows is returned when winp2pns functions are called on non-Windows systems.
	ErrNotWindows = errors.New("winp2pns: this package only works on Windows")
)

// ProxyState represents the current state of a tunnel proxy for a given hostname.
type ProxyState struct {
	// LocalPort is the local port the proxy is currently listening on
	LocalPort uint16
	// IsActive indicates if the tunnel is active. If false, DNS returns NXDOMAIN.
	IsActive bool
}

// TunnelProvider provides real-time tunnel state information.
// Implementations must be safe for concurrent access and return quickly
// to avoid DNS resolution latency.
type TunnelProvider interface {
	// GetProxyState returns the current proxy state for the given hostname.
	// The hostname will be the server portion of the query (e.g., "myserver"
	// from "_minecraft._tcp.myserver.p2p.local").
	GetProxyState(hostname string) (ProxyState, error)
}

// DNSServer represents the running P2P DNS service.
// It binds to localhost and responds to queries for the configured namespace.
type DNSServer struct {
	provider  TunnelProvider
	namespace string // e.g., ".p2p.local"
	nrptGUID  string // GUID for the NRPT registry entry
	server    *dns.Server
	bindAddr  string // Address to bind to (default: "127.0.0.1:53")

	running bool
	mu      sync.RWMutex
}

// Config holds configuration options for the DNS server.
type Config struct {
	// Namespace is the DNS suffix to intercept (e.g., ".p2p.local")
	Namespace string
	// BindAddr is the address to bind to (default: "127.0.0.1:53")
	BindAddr string
	// FallbackAddr is used if BindAddr is unavailable (default: "127.0.0.2:53")
	FallbackAddr string
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Namespace:    ".p2p.local",
		BindAddr:     "127.0.0.1:53",
		FallbackAddr: "127.0.0.2:53",
	}
}

// IsRunning returns whether the DNS server is currently running.
func (s *DNSServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Namespace returns the DNS namespace this server is handling.
func (s *DNSServer) Namespace() string {
	return s.namespace
}

// BindAddress returns the address the server is bound to.
func (s *DNSServer) BindAddress() string {
	return s.bindAddr
}
