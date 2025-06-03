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
- [ ] Group 2: Network Transport
  - [ ] Transport interface defined
  - [ ] TCP implementation
  - [ ] Authentication protocol
  - [ ] TLS upgrade
  - [ ] Unit tests passing
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
┌─────────────┐     TLS + Mesh Protocol    ┌─────────────┐
│  Laptop     │◄──────────────────────────►│  Desktop    │
│ (Client)    │                            │  (Full)     │
└─────┬───────┘                            └──────┬──────┘
      │                                           │
      │                                           │
      │         ┌─────────────┐                   │
      └────────►│   Server    │◄──────────────────┘
                │   (Full)    │
                └─────────────┘
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
- Poll clipboard every 100ms
- Compare hash of contents to detect changes
- Emit only when content actually changes

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

#### Implementation

```go
// pkg/transport/interface.go
type Transport interface {
    // Listen starts accepting connections (server mode)
    Listen(addr string) error
    
    // Connect establishes outbound connection
    Connect(addr string) (*Conn, error)
    
    // Accept returns next incoming connection
    Accept() (*Conn, error)
    
    // Close shuts down transport
    Close() error
}

// pkg/transport/conn.go
type Conn struct {
    ID       string
    Mode     NodeMode
    tls      *tls.Conn
    decoder  *json.Decoder
    encoder  *json.Encoder
}

// pkg/transport/auth.go
type Authenticator interface {
    // Handshake performs shared secret authentication
    Handshake(conn net.Conn, secret string) (*AuthResult, error)
    
    // GenerateTLSConfig creates TLS config after auth
    GenerateTLSConfig(authResult *AuthResult) (*tls.Config, error)
}
```

**Authentication Flow**:
1. TCP connection established
2. Exchange nonce + HMAC(nonce, secret)
3. Verify HMAC matches
4. Upgrade to TLS with generated certs
5. Exchange node information

#### Testing

```go
// Test authentication handshake
func TestHandshake(t *testing.T) {
    auth := NewAuthenticator()
    
    // Create paired connections
    client, server := net.Pipe()
    
    go func() {
        result, err := auth.Handshake(server, "secret", true)
        assert.NoError(t, err)
    }()
    
    result, err := auth.Handshake(client, "secret", false)
    assert.NoError(t, err)
    assert.NotEmpty(t, result.NodeID)
}
```

---

### Group 3: Mesh Topology
**Goal**: Self-organizing network that handles dynamic membership

#### Requirements
- Nodes can join via any existing node
- Full nodes share peer lists
- Client nodes maintain outbound connections only
- Handle node failures and reconnections
- Exponential backoff for reconnections

#### Implementation

```go
// pkg/mesh/topology.go
type Topology struct {
    self    Node
    peers   sync.Map // map[string]*Peer
    pool    *ConnectionPool
    events  chan TopologyEvent
}

type Node struct {
    ID        string
    Mode      NodeMode  // Client or Full
    Addr      string    // Empty for client nodes
    JoinAddrs []string  // Addresses this node can join through
}

type Peer struct {
    Node
    conn      *transport.Conn
    mu        sync.Mutex
    lastSeen  time.Time
    backoff   backoff.BackOff
    
    // Lifecycle management
    ctx       context.Context
    cancel    context.CancelFunc
}

// pkg/mesh/events.go
type TopologyEvent struct {
    Type EventType
    Peer Node
}

type EventType int
const (
    PeerConnected EventType = iota
    PeerDisconnected
    PeerDiscovered
)
```

**Peer Discovery Protocol**:
```json
{
  "type": "peer_exchange",
  "peers": [
    {
      "id": "node-123",
      "mode": "full",
      "addr": "192.168.1.100:8080"
    }
  ]
}
```

#### Testing

```go
func TestMeshFormation(t *testing.T) {
    // Create 3 node topology
    node1 := NewTopology("node1", FullNode, ":0")
    node2 := NewTopology("node2", FullNode, ":0")
    node3 := NewTopology("node3", ClientNode, "")
    
    // Connect in chain
    node2.Join(node1.Addr())
    node3.Join(node2.Addr())
    
    // Verify full mesh formed
    eventually(t, func() bool {
        return node1.PeerCount() == 2 && 
               node2.PeerCount() == 2 &&
               node3.PeerCount() == 2
    })
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
- [ ] Clipboard interface + macOS implementation
- [ ] Linux clipboard implementation  
- [ ] Mock clipboard for testing
- [ ] Basic clipboard watch functionality
- [ ] Unit tests for all clipboard operations

### Phase 2: Networking (Week 1-2)
- [ ] TCP transport implementation
- [ ] Shared secret authentication
- [ ] TLS upgrade after auth
- [ ] Connection wrapper with JSON messaging
- [ ] Transport unit tests

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
- HMAC authentication before TLS handshake
- No persistent storage of clipboard history
- Automatic certificate generation per session

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
