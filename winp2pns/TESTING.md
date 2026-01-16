# Manual Testing Guide: winp2pns

This guide covers how to test the Windows P2P DNS library on a Windows machine.

## Prerequisites

- Windows 10/11
- Administrative privileges (Run as Administrator)
- Go 1.21+ installed
- A Minecraft Java Edition client (for SRV record testing)

## Build

```powershell
# Clone and build
git clone https://github.com/hintjen/wireproxy.git
cd wireproxy
git checkout feature/winp2pns
go build ./cmd/wireproxy
```

## Test 1: NRPT Registration

Verify that the library correctly registers NRPT entries.

### Steps

1. Create a simple test program:

```go
// test_nrpt.go
package main

import (
    "fmt"
    "github.com/pufferffish/wireproxy/winp2pns"
)

func main() {
    // Check admin privileges
    if err := winp2pns.RequireAdmin(); err != nil {
        fmt.Printf("ERROR: %v\n", err)
        fmt.Println("Please run as Administrator")
        return
    }

    // Register namespace
    guid, err := winp2pns.RegisterNamespace(".p2p.local", "127.0.0.1")
    if err != nil {
        fmt.Printf("Failed to register: %v\n", err)
        return
    }
    fmt.Printf("Registered NRPT entry with GUID: %s\n", guid)

    // Verify in registry
    fmt.Println("\nVerify with: reg query \"HKLM\\SOFTWARE\\Policies\\Microsoft\\Windows NT\\DNSClient\\DnsPolicyConfig\"")
    fmt.Println("Press Enter to cleanup...")
    fmt.Scanln()

    // Cleanup
    if err := winp2pns.UnregisterNamespace(guid); err != nil {
        fmt.Printf("Failed to unregister: %v\n", err)
    }
    fmt.Println("Cleanup complete")
}
```

2. Build and run as Administrator:
```powershell
go run test_nrpt.go
```

3. In another terminal, verify the registry entry:
```powershell
reg query "HKLM\SOFTWARE\Policies\Microsoft\Windows NT\DNSClient\DnsPolicyConfig"
```

### Expected Result

- GUID subkey created with:
  - `Name` = `.p2p.local`
  - `GenericDNSServers` = `127.0.0.1`
  - `ConfigOptions` = `1`

---

## Test 2: DNS Server

Test the DNS server responds to queries.

### Steps

1. Create a mock provider:

```go
// test_dns.go
package main

import (
    "fmt"
    "os"
    "os/signal"
    "syscall"
    
    "github.com/pufferffish/wireproxy/winp2pns"
)

type MockProvider struct{}

func (m *MockProvider) GetProxyState(hostname string) (winp2pns.ProxyState, error) {
    fmt.Printf("Query for hostname: %s\n", hostname)
    return winp2pns.ProxyState{
        LocalPort: 25565,
        IsActive:  true,
    }, nil
}

func main() {
    if err := winp2pns.RequireAdmin(); err != nil {
        fmt.Printf("ERROR: %v\nPlease run as Administrator\n", err)
        return
    }

    server, err := winp2pns.New(".p2p.local", &MockProvider{})
    if err != nil {
        fmt.Printf("Failed to create server: %v\n", err)
        return
    }

    // Start in background
    errChan := server.StartAsync()

    fmt.Println("DNS server started on 127.0.0.1:53")
    fmt.Println("Test with: nslookup -type=SRV _minecraft._tcp.myserver.p2p.local 127.0.0.1")
    fmt.Println("Press Ctrl+C to stop...")

    // Wait for signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    select {
    case err := <-errChan:
        fmt.Printf("Server error: %v\n", err)
    case <-sigChan:
        fmt.Println("\nShutting down...")
    }

    server.Stop()
}
```

2. Run as Administrator:
```powershell
go run test_dns.go
```

3. In another terminal, test DNS queries:
```powershell
# Test SRV record
nslookup -type=SRV _minecraft._tcp.myserver.p2p.local 127.0.0.1

# Test A record
nslookup myserver.p2p.local 127.0.0.1
```

### Expected Results

**SRV Query:**
```
Server:  UnKnown
Address:  127.0.0.1

_minecraft._tcp.myserver.p2p.local   SRV service location:
          priority       = 0
          weight         = 0
          port           = 25565
          svr hostname   = tunnel.myserver.p2p.local
```

**A Query:**
```
Server:  UnKnown
Address:  127.0.0.1

Name:    myserver.p2p.local
Address:  127.0.0.1
```

---

## Test 3: Minecraft Java Edition

Test that Minecraft correctly resolves the SRV record.

### Prerequisites

- Minecraft Java Edition installed
- A local TCP server running on port 25565 (or modify the MockProvider)

### Steps

1. Start the DNS server (from Test 2)
2. Start a Minecraft server or mock server on 127.0.0.1:25565
3. Open Minecraft → Multiplayer → Add Server
4. Enter server address: `myserver.p2p.local` (no port!)
5. Click "Done" and attempt to join

### Expected Result

Minecraft should:
1. Query `_minecraft._tcp.myserver.p2p.local` for SRV record
2. Receive port 25565 and target `tunnel.myserver.p2p.local`
3. Query `tunnel.myserver.p2p.local` for A record
4. Connect to `127.0.0.1:25565`

---

## Troubleshooting

### Port 53 Already in Use

If another process is using port 53:

```powershell
# Find what's using port 53
netstat -ano | findstr :53

# The library will fall back to 127.0.0.2:53 automatically
```

### DNS Cache Issues

Clear the DNS cache if you see stale results:

```powershell
ipconfig /flushdns
```

### NRPT Not Taking Effect

1. Verify the registry entry exists
2. Flush DNS cache
3. Restart the DNS Client service:
```powershell
net stop dnscache
net start dnscache
```
