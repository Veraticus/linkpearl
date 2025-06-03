# Linkpearl Architecture Document

## Quick Start for Development

### Directory Structure
```
linkpearl/
├── cmd/
│   └── linkpearl/
│       └── main.go          # CLI entry point
├── pkg/
│   ├── clipboard/           # Platform clipboard access
│   │   ├── interface.go     # Clipboard interface
│   │   ├── darwin.go        # macOS implementation
│   │   ├── linux.go         # Linux implementation
│   │   └── mock.go          # Mock for testing
│   ├── transport/           # Network communication
│   │   ├── interface.go     # Transport interface
│   │   ├── conn.go          # Connection wrapper
│   │   ├── auth.go          # Authentication logic
│   │   └── transport.go     # TCP implementation
│   ├── mesh/                # P2P topology management
│   │   ├── topology.go      # Mesh network logic
│   │   ├── events.go        # Topology events
│   │   └── pool.go          # Connection pool
│   ├── sync/                # Clipboard synchronization
│   │   ├── engine.go        # Sync coordinator
│   │   └── message.go       # Message types
│   └── config/              # Configuration
│       └── config.go        # Config struct and parsing
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── Makefile                 # Build automation
└── README.md               # User documentation
```

### Implementation Dependencies
```
Package Dependencies:
- clipboard: No dependencies (standalone)
- transport: No dependencies (standalone)
- config: No dependencies (standalone)
- mesh: Requires transport
- sync: Requires clipboard, mesh, transport
- cmd: Requires all packages

Implementation Order:
1. config (simplest, needed by all)
2. clipboard (core functionality, testable in isolation)
3. transport (networking foundation)
4. mesh (builds on transport)
5. sync (integrates everything)
6. cmd (final CLI assembly)
```

### Test Commands
```bash
# Run all unit tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test ./pkg/clipboard
go test ./pkg/transport

# Run with coverage
go test -cover ./...

# Run integration tests (requires build tag)
go test -tags=integration ./...

# Run benchmarks
go test -bench=. ./...

# Test a specific function
go test -run TestClipboardWatch ./pkg/clipboard
```

### Common Development Commands
```bash
# Initialize project
go mod init github.com/yourusername/linkpearl

# Add dependencies as needed
go get -u golang.org/x/crypto/nacl/box

# Format code
go fmt ./...

# Lint code (requires golangci-lint)
golangci-lint run

# Build binary
go build -o linkpearl ./cmd/linkpearl

# Run locally
./linkpearl --secret mysecret --listen :8080
```

## Current Status
- [x] Project initialized (go.mod created)
- [x] Directory structure created
- [x] Makefile added
- [x] Config package implemented and tested
- [x] Group 1: Clipboard Interface
  - [x] Interface defined (clipboard/interface.go)
  - [x] macOS implementation (clipboard/darwin.go)
  - [x] Linux implementation (clipboard/linux.go)
  - [x] Mock implementation (clipboard/mock.go)
  - [x] Unit tests passing
- [x] Group 2: Network Transport
  - [x] Transport interface defined
  - [x] TCP implementation
  - [x] Authentication protocol
  - [x] TLS upgrade
  - [x] Unit tests passing
- [ ] Group 3: Mesh Topology
  - [ ] Topology manager
  - [ ] Peer discovery
  - [ ] Connection pool
  - [ ] Reconnection logic
  - [ ] Integration tests passing
- [ ] Group 4: Sync Engine
  - [ ] Sync engine core
  - [ ] Message deduplication
  - [ ] Conflict resolution
  - [ ] End-to-end tests passing
- [ ] Group 5: CLI & Config
  - [ ] Config parsing
  - [ ] CLI flags
  - [ ] Main entry point
  - [ ] Graceful shutdown

## Overview

Linkpearl is a secure, peer-to-peer clipboard synchronization tool that creates a mesh network of computers, allowing seamless clipboard sharing between machines. Named after Final Fantasy XIV's communication crystals, Linkpearl connects your devices in a "linkshell" for instant clipboard synchronization.

### Core Principles
- **Simplicity**: Small binary, minimal dependencies, easy setup
- **Security**: All clipboard data encrypted in transit via TLS
- **Resilience**: Handles network disruptions, sleeping computers, and transient connections
- **Privacy**: Your data never touches third-party servers
- **Cross-platform**: Native support for macOS and Linux (Windows later)

### High-Level Architecture

```
┌─────────────┐     TLS + Static Overlay    ┌─────────────┐
│  Laptop     │◄──────────────────────────►│  Desktop    │
│ (Client)    │                            │  (Full)     │
└─────────────┘                            └─────────────┘
      ↑                                           ↑
      │                                           │
      │         ┌─────────────┐                   │
      └─────────│   Server    │───────────────────┘
                │   (Full)    │
                └─────────────┘

Legend:
- Solid lines: Persistent connections based on --join configuration
- Client nodes: Only make outbound connections
- Full nodes: Accept incoming connections + make configured outbound connections
- No automatic peer discovery or mesh formation
```

## Feature Groups

### Group 1: Clipboard Interface
**Goal**: Platform-agnostic clipboard access

#### Requirements
- Read current clipboard contents
- Write new clipboard contents  
- Watch for clipboard changes
- Support text content (MVP)
- Work on macOS and Linux (with varying clipboard providers, xsel, xclip, etc.)

#### Implementation

```go
// pkg/clipboard/interface.go
type Clipboard interface {
    // Read returns current clipboard contents
    Read() (string, error)
    
    // Write sets clipboard contents
    Write(content string) error
    
    // Watch returns channel that emits on clipboard changes
    // Implementation varies by platform:
    // - Linux: Event-based via XFixes
    // - Windows: Event-based via WM_CLIPBOARDUPDATE
    // - macOS: Smart polling with changeCount
    Watch(ctx context.Context) <-chan string
}}

// Platform-specific implementations
// pkg/clipboard/darwin.go  - Uses pbcopy/pbpaste
// pkg/clipboard/linux/xsel.go - Linux clipboard providers, this one is xsel
// pkg/clipboard/mock.go    - For testing

func (c *DarwinClipboard) Watch(ctx context.Context) <-chan string {
    ch := make(chan string)
    go func() {
        defer close(ch)
        
        lastChangeCount := c.getChangeCount()
        ticker := time.NewTicker(500 * time.Millisecond) // Longer interval
        defer ticker.Stop()
        
        idleCount := 0
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                currentCount := c.getChangeCount()
                if currentCount != lastChangeCount {
                    lastChangeCount = currentCount
                    idleCount = 0
                    if content, err := c.Read(); err == nil {
                        ch <- content
                    }
                } else {
                    idleCount++
                    // Slow down polling when idle
                    if idleCount > 10 {
                        ticker.Reset(2 * time.Second)
                    }
                }
            }
        }
    }()
    return ch
}

// pkg/clipboard/linux.go
func (c *LinuxClipboard) Watch(ctx context.Context) <-chan string {
    ch := make(chan string)
    go func() {
        defer close(ch)
        
        // Option 1: Use clipnotify if available
        if _, err := exec.LookPath("clipnotify"); err == nil {
            c.watchWithClipnotify(ctx, ch)
            return
        }
        
        // Option 2: Fall back to smart polling
        c.watchWithPolling(ctx, ch)
    }()
    return ch
}

func (c *LinuxClipboard) watchWithClipnotify(ctx context.Context, ch chan<- string) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            // clipnotify blocks until clipboard changes
            cmd := exec.CommandContext(ctx, "clipnotify")
            if err := cmd.Run(); err != nil {
                time.Sleep(time.Second)
                continue
            }
            
            // Now read the actual content
            if content, err := c.Read(); err == nil {
                ch <- content
            }
        }
    }
}

func (c *LinuxClipboard) Read() (string, error) {
    // Try different clipboard tools in order
    tools := []struct {
        name string
        args []string
    }{
        {"wl-paste", []string{"--no-newline"}},              // Wayland
        {"xsel", []string{"--output", "--clipboard"}},       // X11
        {"xclip", []string{"-out", "-selection", "clipboard"}}, // X11
    }
    
    for _, tool := range tools {
        if _, err := exec.LookPath(tool.name); err == nil {
            cmd := exec.Command(tool.name, tool.args...)
            output, err := cmd.Output()
            if err == nil {
                return string(output), nil
            }
        }
    }
    
    return "", fmt.Errorf("no clipboard tool found")
}
```

**Watch Implementation Strategy**:
- macOS: Uses changeCount API via AppleScript for efficient change detection
- Linux: Prefers clipnotify if available, falls back to smart polling
- Adaptive polling: Speeds up on changes (500ms), slows down when idle (2s)
- SHA256 content hashing to detect actual changes vs. duplicate events

#### Testing

```go
// Unit tests with mock
func TestClipboardWatch(t *testing.T) {
    mock := NewMockClipboard()
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    ch := mock.Watch(ctx)
    
    mock.Write("test")
    
    select {
    case content := <-ch:
        assert.Equal(t, "test", content)
    case <-time.After(time.Second):
        t.Fatal("timeout waiting for clipboard change")
    }
}

// Integration test (build tag)
// +build integration
func TestRealClipboard(t *testing.T) {
    clip := NewPlatformClipboard()
    // Test with actual system clipboard
}
```

---

### Group 2: Network Transport
**Goal**: Secure, authenticated connections between nodes

#### Requirements
- TCP-based communication
- TLS encryption after authentication
- Shared secret authentication
- Support both server and client modes
- Handle connection drops gracefully

#### Design Decisions
- **TLS Certificates**: Use ephemeral self-signed certificates generated on startup. No CA complexity needed since we authenticate with shared secret.
- **Authentication Protocol**: Simultaneous nonce exchange with timestamps for replay protection (5-minute window)
- **Node Information**: Exchange NodeID, Mode, and Version after TLS upgrade
- **Connection Management**: No pooling at transport layer; basic TCP keepalives (30s); 10s handshake timeout
- **Error Handling**: Close immediately on auth failure; generic errors to remote; detailed errors in local logs only

#### Implementation Deviations and Details
- **Certificate Generation**: Using RSA 2048-bit keys (standard, fast generation) valid for 24 hours
- **Conn Interface**: Changed from struct to interface for better testability and abstraction
- **Transport Interface**: Connect() takes a context for cancellation/timeout control
- **Connection Type**: secureConn uses net.Conn interface internally (not just *tls.Conn) for testing flexibility
- **Message Size Limit**: Added 1MB limit on incoming messages to prevent memory exhaustion
- **Logger Interface**: Added optional logging support with no-op default for zero dependencies
- **Node ID Generation**: Using hostname + nanosecond timestamp for uniqueness
- **Concurrent Handshake**: Client and server send auth messages simultaneously (not challenge-response)

#### Implementation

```go
// pkg/transport/interface.go
type Transport interface {
    // Listen starts accepting connections (server mode)
    Listen(addr string) error
    
    // Connect establishes outbound connection
    Connect(ctx context.Context, addr string) (Conn, error)
    
    // Accept returns next incoming connection
    Accept() (Conn, error)
    
    // Close shuts down transport
    Close() error
    
    // Addr returns the listener address (nil if not listening)
    Addr() net.Addr
}

// Conn is now an interface (not a struct)
type Conn interface {
    NodeID() string
    Mode() string
    Version() string
    Send(msg interface{}) error
    Receive(msg interface{}) error
    Close() error
    LocalAddr() net.Addr
    RemoteAddr() net.Addr
    SetDeadline(t time.Time) error
    SetReadDeadline(t time.Time) error
    SetWriteDeadline(t time.Time) error
}

// pkg/transport/auth.go
type Authenticator interface {
    // Handshake performs shared secret authentication
    Handshake(conn net.Conn, secret string, isServer bool) (*AuthResult, error)
    
    // GenerateTLSConfig creates TLS config after auth
    GenerateTLSConfig(isServer bool) (*tls.Config, error)
}

// Authentication message format
type AuthMessage struct {
    Nonce     string `json:"nonce"`
    Timestamp int64  `json:"timestamp"`
    HMAC      string `json:"hmac"` // HMAC(nonce+timestamp+secret)
}

// Node info exchanged after TLS upgrade
type NodeInfo struct {
    NodeID  string   `json:"node_id"`
    Mode    NodeMode `json:"mode"`
    Version string   `json:"version"`
}
```

**Authentication Flow**:
1. TCP connection established
2. Both sides simultaneously send AuthMessage with nonce, timestamp, and HMAC
3. Verify HMAC matches: `HMAC-SHA256(nonce+timestamp+secret)`
4. Verify timestamp is within 5 minutes of current time
5. Upgrade to TLS 1.3 with ephemeral self-signed certificates
6. Exchange NodeInfo for version compatibility and mesh formation

#### Testing Approach

- **Unit Tests**: Mock connections using net.Pipe() for isolated component testing
- **Integration Tests**: Full TCP server/client with real TLS handshakes
- **Test Coverage**: Authentication, connection lifecycle, error cases, concurrent connections
- **Skipped Tests**: Some I/O-heavy tests skipped to avoid timeouts (marked for future refactoring)

```go
// Example test structure
func TestAuthenticator(t *testing.T) {
    t.Run("CreateAuthMessage", func(t *testing.T) { /* Test HMAC generation */ })
    t.Run("VerifyAuthMessage", func(t *testing.T) { /* Test timestamp validation */ })
    t.Run("Handshake", func(t *testing.T) { /* Test full handshake */ })
    t.Run("HandshakeWithWrongSecret", func(t *testing.T) { /* Test auth failure */ })
    t.Run("TLSConfig", func(t *testing.T) { /* Test certificate generation */ })
}
```

---

### Group 3: Mesh Topology
**Goal**: Static overlay network with persistent connections

#### Requirements
- Nodes maintain persistent connections to their configured peers
- Full nodes accept incoming connections and maintain them
- Client nodes maintain outbound connections only
- Handle node failures with automatic reconnection
- Exponential backoff for reconnections

#### Design Decisions
- **Static Topology**: Nodes only connect to addresses from `--join` flags, never to discovered peers
- **Persistent Connections**: All connections are maintained persistently for real-time sync
- **Peer Exchange**: Informational only - nodes share their peer lists but never act on them
- **No Peer Hopping**: If nodes can't communicate directly, they don't try alternate routes
- **Reconnection Strategy**: 
  - Initial retry: 1 second
  - Max backoff: 5 minutes
  - Backoff factor: 2x with ±10% jitter
  - Retry forever (never remove configured peers)
- **Event Buffer**: 1000 events max, ring buffer with oldest dropped on overflow
- **Simple Architecture**: No routing, no consensus, predictable message flow

#### Implementation

```go
// pkg/mesh/topology.go
type Topology struct {
    self      Node
    transport transport.Transport
    
    // Persistent connections
    mu        sync.RWMutex
    peers     map[string]*Peer    // Connected peers by node ID
    joinAddrs []string            // Configured addresses to maintain connections to
    
    // Event system
    events    chan TopologyEvent  // Buffered channel (1000 events)
    
    // Lifecycle
    ctx       context.Context
    cancel    context.CancelFunc
}

type Node struct {
    ID        string
    Mode      string    // "client" or "full"
    Addr      string    // Listen address (empty for client nodes)
}

type Peer struct {
    Node
    conn      transport.Conn
    
    // For configured peers (from --join)
    addr      string              // Address to reconnect to
    reconnect bool                // Should we reconnect if disconnected?
    
    // Reconnection state
    mu        sync.Mutex
    backoff   *ExponentialBackoff
    
    // Lifecycle
    ctx       context.Context
    cancel    context.CancelFunc
}

// Exponential backoff configuration
type ExponentialBackoff struct {
    current   time.Duration
    max       time.Duration
    factor    float64
    jitter    float64
}

// pkg/mesh/events.go
type TopologyEvent struct {
    Type EventType
    Peer Node
    Time time.Time
}

type EventType int
const (
    PeerConnected EventType = iota
    PeerDisconnected
    // Note: No PeerDiscovered - topology is static
)

// Ring buffer for events
type EventBuffer struct {
    events [1000]TopologyEvent
    head   int
    tail   int
    size   int
    mu     sync.Mutex
}
```

**Peer Exchange Protocol**:
```json
{
  "type": "peer_list",
  "from": "node-123",
  "peers": [
    {
      "id": "node-456",
      "mode": "full",
      "addr": "192.168.1.100:8080"
    },
    {
      "id": "node-789", 
      "mode": "client",
      "addr": ""  // Client nodes have no listen address
    }
  ]
}
```

Note: Peer lists are exchanged on connect/disconnect for observability only. Nodes never use this information to form new connections.

#### Testing

```go
func TestStaticTopology(t *testing.T) {
    // Create nodes
    node1 := NewTopology("node1", "full", ":0")
    node2 := NewTopology("node2", "full", ":0")
    node3 := NewTopology("node3", "client", "")
    
    // Start node1 listening
    node1.Start()
    addr1 := node1.ListenAddr()
    
    // Node2 connects to node1
    node2.AddJoinAddr(addr1)
    node2.Start()
    
    // Node3 (client) connects to node1
    node3.AddJoinAddr(addr1)
    node3.Start()
    
    // Verify connections
    // - node1 has 2 incoming connections
    // - node2 has 1 outgoing connection  
    // - node3 has 1 outgoing connection
    eventually(t, func() bool {
        return node1.PeerCount() == 2 && 
               node2.PeerCount() == 1 &&
               node3.PeerCount() == 1
    })
}

func TestReconnection(t *testing.T) {
    // Test that connections are re-established after failure
    node1 := NewTopology("node1", "full", ":0")
    node2 := NewTopology("node2", "client", "")
    
    node1.Start()
    node2.AddJoinAddr(node1.ListenAddr())
    node2.Start()
    
    // Simulate connection drop
    node1.DisconnectPeer("node2")
    
    // Verify reconnection with exponential backoff
    eventually(t, func() bool {
        return node2.IsConnected("node1")
    }, 10*time.Second)
}
```

---

### Group 4: Sync Engine
**Goal**: Coordinate clipboard synchronization across the mesh

#### Requirements
- Broadcast local clipboard changes
- Apply remote clipboard changes
- Handle conflicts (last-write-wins)
- Deduplicate messages
- Prevent sync loops

#### Implementation

```go
// pkg/sync/engine.go
type SyncEngine struct {
    clipboard  clipboard.Clipboard
    topology   *mesh.Topology
    
    // Deduplication
    seen       *lru.Cache // [checksum]timestamp
    
    // Current state
    mu         sync.Mutex
    current    string
    checksum   string
    timestamp  int64
}

// pkg/sync/message.go
type ClipboardMessage struct {
    NodeID    string `json:"node_id"`
    Timestamp int64  `json:"timestamp"`
    Checksum  string `json:"checksum"`
    Content   string `json:"content"`
}

func (e *SyncEngine) Run(ctx context.Context) error {
    // Watch local clipboard
    clipCh := e.clipboard.Watch(ctx)
    
    // Watch topology events
    eventCh := e.topology.Events()
    
    // Watch incoming messages
    msgCh := e.topology.Messages()
    
    for {
        select {
        case content := <-clipCh:
            e.handleLocalChange(content)
            
        case msg := <-msgCh:
            e.handleRemoteChange(msg)
            
        case event := <-eventCh:
            e.handleTopologyEvent(event)
            
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

**Conflict Resolution**:
- Last-write-wins based on timestamp
- Nanosecond precision to minimize conflicts
- Node ID as tiebreaker

#### Testing

```go
func TestSyncEngine(t *testing.T) {
    mockClip := clipboard.NewMock()
    mockTopo := mesh.NewMockTopology()
    
    engine := NewSyncEngine(mockClip, mockTopo)
    
    // Simulate local change
    mockClip.EmitChange("local content")
    
    // Verify broadcast
    msg := mockTopo.GetLastBroadcast()
    assert.Equal(t, "local content", msg.Content)
    
    // Simulate remote change
    mockTopo.EmitMessage(ClipboardMessage{
        NodeID:    "remote",
        Content:   "remote content",
        Timestamp: time.Now().UnixNano(),
    })
    
    // Verify clipboard updated
    assert.Equal(t, "remote content", mockClip.Read())
}
```

---

### Group 5: Configuration & CLI
**Goal**: Simple, flexible configuration

#### Requirements
- CLI flags for all options
- Environment variable support
- No configuration files (MVP)
- Sensible defaults

#### Implementation

```go
// pkg/config/config.go
type Config struct {
    // Core settings
    Secret    string   `env:"LINKPEARL_SECRET"`
    NodeID    string   `env:"LINKPEARL_NODE_ID"`
    
    // Network settings  
    Listen    string   `env:"LINKPEARL_LISTEN"`
    Join      []string `env:"LINKPEARL_JOIN"`
    
    // Behavior
    PollInterval time.Duration `env:"LINKPEARL_POLL_INTERVAL"`
    Verbose      bool          `env:"LINKPEARL_VERBOSE"`
}

// cmd/linkpearl/main.go
func main() {
    var cfg config.Config
    
    flag.StringVar(&cfg.Secret, "secret", "", "Shared secret for linkshell")
    flag.StringVar(&cfg.Listen, "listen", "", "Listen address (e.g. :8080)")
    flag.Var(&cfg.Join, "join", "Address to join (can be repeated)")
    flag.BoolVar(&cfg.Verbose, "v", false, "Verbose logging")
    
    flag.Parse()
    
    // Validate
    if cfg.Secret == "" {
        log.Fatal("--secret is required")
    }
    
    // Start
    if err := run(cfg); err != nil {
        log.Fatal(err)
    }
}
```

---

## Development Roadmap

### Phase 1: Foundation (Week 1)
- [x] Clipboard interface + macOS implementation
- [x] Linux clipboard implementation  
- [x] Mock clipboard for testing
- [x] Basic clipboard watch functionality
- [x] Unit tests for all clipboard operations

### Phase 2: Networking (Week 1-2)
- [x] TCP transport implementation
- [x] Shared secret authentication
- [x] TLS upgrade after auth
- [x] Connection wrapper with JSON messaging
- [x] Transport unit tests

### Phase 3: Mesh (Week 2)
- [ ] Node types (Client/Full)
- [ ] Connection pool with reconnection
- [ ] Peer discovery via exchange
- [ ] Topology event system
- [ ] Integration tests for mesh formation

### Phase 4: Sync (Week 3)
- [ ] Sync engine core loop
- [ ] Message deduplication
- [ ] Conflict resolution
- [ ] Loop prevention
- [ ] End-to-end sync tests

### Phase 5: Polish (Week 3-4)
- [ ] CLI interface
- [ ] Logging and debugging
- [ ] Graceful shutdown
- [ ] Binary releases
- [ ] Documentation

## Testing Strategy

### Unit Tests
- Mock all interfaces
- Test each component in isolation
- Focus on edge cases and error handling

### Integration Tests  
- Use build tags to separate from unit tests
- Test real clipboard operations
- Test network formation with real sockets
- Test full sync flow with multiple nodes

### Manual Testing Scenarios
1. **Happy path**: 2 nodes, copy on one, paste on other
2. **Transient node**: Laptop sleep/wake cycle
3. **Network partition**: Disconnect and reconnect
4. **Conflict**: Simultaneous copies on different nodes
5. **Scale**: 5+ nodes in various configurations

## Security Considerations

### Threat Model
- **Attacker on network**: Can see encrypted traffic but not contents
- **Without secret**: Cannot join linkshell or decrypt traffic
- **With secret**: Trusted member of linkshell

### Mitigations
- TLS 1.3 for all data transmission
- HMAC-SHA256 authentication before TLS handshake
- Timestamp validation (5-minute window) prevents replay attacks
- No persistent storage of clipboard history
- Ephemeral self-signed certificates generated per session
- Auth failures result in immediate connection termination

## Future Enhancements

### Post-MVP Features
1. **Windows support**
2. **Image support** for screenshots
3. **Clipboard history** with navigation
4. **System tray** integration
5. **Hot keys** for enable/disable
6. **Persistent peer** discovery cache
7. **NAT traversal** via STUN/TURN

This architecture provides a solid foundation for building Linkpearl incrementally, with clear boundaries between components and comprehensive testing at each layer.
