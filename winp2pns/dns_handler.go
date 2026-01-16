package winp2pns

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"
)

// Common errors
var (
	ErrAlreadyRunning = errors.New("winp2pns: DNS server is already running")
	ErrNotRunning     = errors.New("winp2pns: DNS server is not running")
	ErrPortInUse      = errors.New("winp2pns: port 53 is already in use")
)

// New creates a new DNS server for the given namespace.
// The provider must implement TunnelProvider to supply real-time proxy state.
func New(namespace string, provider TunnelProvider) (*DNSServer, error) {
	return NewWithConfig(Config{
		Namespace:    namespace,
		BindAddr:     "127.0.0.1:53",
		FallbackAddr: "127.0.0.2:53",
	}, provider)
}

// NewWithConfig creates a new DNS server with custom configuration.
func NewWithConfig(cfg Config, provider TunnelProvider) (*DNSServer, error) {
	if provider == nil {
		return nil, errors.New("winp2pns: provider cannot be nil")
	}

	// Ensure namespace starts with a dot
	namespace := cfg.Namespace
	if !strings.HasPrefix(namespace, ".") {
		namespace = "." + namespace
	}

	return &DNSServer{
		provider:  provider,
		namespace: namespace,
		bindAddr:  cfg.BindAddr,
	}, nil
}

// Start begins listening for DNS queries and registers the NRPT rule.
// This method blocks until Stop is called.
func (s *DNSServer) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	s.mu.Unlock()

	// Check for admin privileges on Windows
	if err := RequireAdmin(); err != nil && err != ErrNotWindows {
		return fmt.Errorf("winp2pns: %w", err)
	}

	// Try to bind to the primary address
	bindAddr := s.bindAddr
	conn, err := net.ListenPacket("udp", bindAddr)
	if err != nil {
		// Try fallback address
		bindAddr = "127.0.0.2:53"
		conn, err = net.ListenPacket("udp", bindAddr)
		if err != nil {
			return fmt.Errorf("winp2pns: failed to bind to port 53: %w", err)
		}
	}
	conn.Close() // We'll let dns.Server rebind

	// Extract just the IP for NRPT registration
	host, _, _ := net.SplitHostPort(bindAddr)

	// Register NRPT rule (Windows only, stub returns error on other platforms)
	guid, err := RegisterNamespace(s.namespace, host)
	if err != nil && err != ErrNotWindows {
		return fmt.Errorf("winp2pns: failed to register NRPT: %w", err)
	}
	s.nrptGUID = guid

	// Flush DNS cache so the new rule takes effect
	_ = FlushDNSCache()

	// Create DNS server
	s.server = &dns.Server{
		Addr:    bindAddr,
		Net:     "udp",
		Handler: dns.HandlerFunc(s.handleQuery),
	}

	s.mu.Lock()
	s.running = true
	s.bindAddr = bindAddr
	s.mu.Unlock()

	log.Printf("winp2pns: DNS server started on %s for namespace %s", bindAddr, s.namespace)

	// This blocks until Shutdown is called
	return s.server.ListenAndServe()
}

// StartAsync starts the DNS server in a goroutine and returns immediately.
// Returns a channel that will receive an error if the server fails to start.
func (s *DNSServer) StartAsync() <-chan error {
	errChan := make(chan error, 1)
	go func() {
		err := s.Start()
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()
	return errChan
}

// Stop gracefully shuts down the DNS server and cleans up NRPT entries.
func (s *DNSServer) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	var errs []error

	// Shutdown DNS server
	if s.server != nil {
		if err := s.server.Shutdown(); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown DNS server: %w", err))
		}
	}

	// Unregister NRPT rule
	if s.nrptGUID != "" {
		if err := UnregisterNamespace(s.nrptGUID); err != nil && err != ErrNotWindows {
			errs = append(errs, fmt.Errorf("failed to unregister NRPT: %w", err))
		}
		s.nrptGUID = ""
	}

	// Flush DNS cache
	_ = FlushDNSCache()

	log.Printf("winp2pns: DNS server stopped")

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// handleQuery processes incoming DNS queries.
func (s *DNSServer) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		name := q.Name
		nameLower := strings.ToLower(name)

		// Check if this query is for our namespace
		if !strings.HasSuffix(strings.TrimSuffix(nameLower, "."), s.namespace) {
			// Not our namespace - refuse
			m.Rcode = dns.RcodeRefused
			w.WriteMsg(m)
			return
		}

		switch q.Qtype {
		case dns.TypeSRV:
			s.handleSRVQuery(m, q)
		case dns.TypeA:
			s.handleAQuery(m, q)
		case dns.TypeAAAA:
			// Return empty response for IPv6 (we only support IPv4 localhost)
			// No error, just no records
		default:
			// Unsupported query type
			m.Rcode = dns.RcodeNotImplemented
		}
	}

	w.WriteMsg(m)
}

// handleSRVQuery handles SRV record queries (e.g., _minecraft._tcp.myserver.p2p.local)
func (s *DNSServer) handleSRVQuery(m *dns.Msg, q dns.Question) {
	name := strings.TrimSuffix(q.Name, ".")

	// Parse the SRV query name: _service._proto.hostname.namespace
	// For Minecraft: _minecraft._tcp.myserver.p2p.local
	parts := strings.Split(name, ".")
	if len(parts) < 4 {
		m.Rcode = dns.RcodeNameError
		return
	}

	// Extract service, proto, and hostname
	service := parts[0]
	// proto := parts[1]

	// Find where the namespace starts
	namespaceWithoutDot := strings.TrimPrefix(s.namespace, ".")
	namespaceParts := strings.Split(namespaceWithoutDot, ".")

	// The hostname is everything between the service/proto and the namespace
	hostnameEndIdx := len(parts) - len(namespaceParts)
	if hostnameEndIdx < 2 {
		m.Rcode = dns.RcodeNameError
		return
	}
	hostname := strings.Join(parts[2:hostnameEndIdx], ".")

	// Look up the proxy state
	state, err := s.provider.GetProxyState(hostname)
	if err != nil || !state.IsActive {
		m.Rcode = dns.RcodeNameError
		return
	}

	// Construct the target hostname for the SRV record
	targetHost := fmt.Sprintf("tunnel.%s%s.", hostname, s.namespace)

	// Add SRV record
	srv := &dns.SRV{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeSRV,
			Class:  dns.ClassINET,
			Ttl:    0, // Zero TTL for instant failover
		},
		Priority: 0,
		Weight:   0,
		Port:     state.LocalPort,
		Target:   targetHost,
	}
	m.Answer = append(m.Answer, srv)

	// Add A record for the target in the Additional section
	a := &dns.A{
		Hdr: dns.RR_Header{
			Name:   targetHost,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    0,
		},
		A: net.ParseIP("127.0.0.1"),
	}
	m.Extra = append(m.Extra, a)

	log.Printf("winp2pns: SRV query for %s (service=%s, host=%s) -> port %d",
		name, service, hostname, state.LocalPort)
}

// handleAQuery handles A record queries
func (s *DNSServer) handleAQuery(m *dns.Msg, q dns.Question) {
	name := strings.TrimSuffix(q.Name, ".")

	// For A queries, we always return 127.0.0.1 if it's in our namespace
	// The actual port routing is handled by SRV records

	// Add A record
	a := &dns.A{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    0, // Zero TTL for instant failover
		},
		A: net.ParseIP("127.0.0.1"),
	}
	m.Answer = append(m.Answer, a)

	log.Printf("winp2pns: A query for %s -> 127.0.0.1", name)
}
