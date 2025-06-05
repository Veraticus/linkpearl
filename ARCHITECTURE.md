# Linkpearl Architecture Document

## Overview

Linkpearl is a secure, peer-to-peer clipboard synchronization tool that creates a mesh network of computers, allowing seamless clipboard sharing between machines. Named after Final Fantasy XIV's communication crystals, Linkpearl connects your devices in a "linkshell" for instant clipboard synchronization.

### Core Principles
- **Simplicity**: Small binary, minimal dependencies, easy setup
- **Security**: All clipboard data encrypted in transit via TLS
- **Resilience**: Handles network disruptions, sleeping computers, and transient connections
- **Privacy**: Your data never touches third-party servers
- **Cross-platform**: Native support for macOS and Linux (Windows later)

## Critical Architecture Decisions

### Clipboard Event Handling: State-Based Synchronization

**Decision**: Clipboards are treated as **state** rather than **event streams**. The Watch interface returns notifications that state has changed, not the actual content.

**Rationale**: 
- Prevents blocking when sync engine is busy processing
- Avoids unbounded memory growth from queued clipboard data
- Natural deduplication of rapid clipboard changes
- Clear separation between notification and data transfer

**Implementation**:
```go
type Clipboard interface {
    Read() (string, error)
    Write(content string) error
    
    // Watch returns a channel that signals when clipboard state changes.
    // The channel receives empty structs as notifications.
    // Actual content must be retrieved using Read().
    // Channel has buffer size of 10 to handle bursts without blocking.
    Watch(ctx context.Context) <-chan struct{}
}
```

**Key Properties**:
1. **Non-blocking**: Watchers never block on clipboard data
2. **Memory-bounded**: No queuing of clipboard contents
3. **Pull-based**: Sync engine reads when ready, not when pushed
4. **Coalescing**: Multiple rapid changes may produce single notification

**Sequence Tracking**: Each clipboard implementation maintains:
- `sequenceNumber`: Monotonically increasing counter for each change
- `lastModified`: Timestamp of last change
- These help detect missed changes during processing delays

### Event Flow Architecture

```
Clipboard Change
    ↓
Watcher detects change
    ↓
Send notification (struct{}) → [buffered chan, size 10] → Sync Engine
    ↓                                                          ↓
Returns immediately                                    Processes when ready
                                                              ↓
                                                      Pulls current state
                                                              ↓
                                                      Broadcasts to peers
```

This architecture ensures:
- Clipboard watchers never block on slow consumers
- Memory usage is bounded regardless of clipboard activity
- System remains responsive under load
- Natural rate limiting through state pulling

### Benefits of State-Based Design

1. **Resilience**: If sync engine is temporarily busy (network issues, CPU spike), clipboard monitoring continues unaffected
2. **Natural Deduplication**: Rapid clipboard changes (e.g., user selecting text repeatedly) collapse into single sync operation
3. **Predictable Memory**: No risk of unbounded growth from queued clipboard contents
4. **Simpler Testing**: Can test notification and data retrieval separately
5. **Better Debugging**: Sequence numbers and timestamps make it easy to track missed updates

## Directory Structure
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
**Goal**: Platform-agnostic clipboard access with state-based synchronization

#### Requirements
- Read current clipboard contents
- Write new clipboard contents  
- Watch for clipboard changes (notification-based)
- Support text content (MVP)
- Work on macOS and Linux (with varying clipboard providers, xsel, xclip, etc.)
- Non-blocking event notification
- Bounded memory usage

#### Implementation

```go
// pkg/clipboard/interface.go
type Clipboard interface {
    // Read returns current clipboard contents
    Read() (string, error)
    
    // Write sets clipboard contents
    Write(content string) error
    
    // Watch returns channel that signals when clipboard state changes.
    // The channel receives empty structs as notifications.
    // Actual content must be retrieved using Read().
    // Channel has buffer size of 10 to handle bursts without blocking.
    // Multiple rapid changes may result in a single notification.
    Watch(ctx context.Context) <-chan struct{}
    
    // GetState returns current state information
    GetState() ClipboardState
}

// ClipboardState provides metadata about clipboard state
type ClipboardState struct {
    SequenceNumber uint64    // Monotonically increasing counter
    LastModified   time.Time // When clipboard was last changed
    ContentHash    string    // SHA256 hash of current content
}

// Platform-specific implementations
// pkg/clipboard/darwin.go  - Uses pbcopy/pbpaste
// pkg/clipboard/linux/xsel.go - Linux clipboard providers, this one is xsel
// pkg/clipboard/mock.go    - For testing

func (c *DarwinClipboard) Watch(ctx context.Context) <-chan struct{} {
    ch := make(chan struct{}, 10) // Buffered to handle bursts
    go func() {
        defer close(ch)
        
        lastChangeCount := c.getChangeCount()
        ticker := time.NewTicker(500 * time.Millisecond)
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
                    c.sequenceNumber.Add(1)
                    c.lastModified = time.Now()
                    
                    // Send notification (non-blocking)
                    select {
                    case ch <- struct{}{}:
                    default:
                        // Channel full, skip notification
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
func (c *LinuxClipboard) Watch(ctx context.Context) <-chan struct{} {
    ch := make(chan struct{}, 10) // Buffered to handle bursts
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

func (c *LinuxClipboard) watchWithClipnotify(ctx context.Context, ch chan<- struct{}) {
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
            
            c.sequenceNumber.Add(1)
            c.lastModified = time.Now()
            
            // Send notification (non-blocking)
            select {
            case ch <- struct{}{}:
            default:
                // Channel full, skip notification
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
    
    // Write triggers notification
    mock.Write("test")
    
    // Wait for notification
    select {
    case <-ch:
        // Notification received, now read content
        content, err := mock.Read()
        assert.NoError(t, err)
        assert.Equal(t, "test", content)
        
        // Check state
        state := mock.GetState()
        assert.Equal(t, uint64(1), state.SequenceNumber)
    case <-time.After(time.Second):
        t.Fatal("timeout waiting for clipboard change notification")
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
    // Watch local clipboard for notifications
    clipCh := e.clipboard.Watch(ctx)
    
    // Watch topology events
    eventCh := e.topology.Events()
    
    // Watch incoming messages
    msgCh := e.topology.Messages()
    
    for {
        select {
        case <-clipCh:
            // Notification received - pull current state
            content, err := e.clipboard.Read()
            if err != nil {
                e.logger.Error("failed to read clipboard", "error", err)
                continue
            }
            state := e.clipboard.GetState()
            e.handleLocalChange(content, state)
            
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

## Production Hardening Guidelines

### Command Execution Hardening

**Decision**: All external command executions must have timeouts, proper resource cleanup, and comprehensive error handling.

**Rationale**: External commands can hang indefinitely, leak resources, or fail in unexpected ways. Production systems need predictable behavior and resource management.

**Implementation**:

#### 1. Command Timeout Pattern

All clipboard command executions should follow this pattern:

```go
// clipboard/command.go - Shared command execution utilities
package clipboard

import (
    "context"
    "fmt"
    "os/exec"
    "time"
)

// CommandTimeout is the maximum time allowed for clipboard operations
const CommandTimeout = 5 * time.Second

// CommandConfig holds configuration for command execution
type CommandConfig struct {
    // Timeout for command execution (default: CommandTimeout)
    Timeout time.Duration
    
    // MaxOutputSize limits the amount of data read (default: MaxClipboardSize)
    MaxOutputSize int
    
    // Logger for debugging command execution
    Logger func(format string, args ...interface{})
}

// DefaultCommandConfig returns config with production-ready defaults
func DefaultCommandConfig() *CommandConfig {
    return &CommandConfig{
        Timeout:       CommandTimeout,
        MaxOutputSize: MaxClipboardSize,
        Logger:        func(string, ...interface{}) {}, // no-op by default
    }
}

// RunCommand executes a command with proper timeout and resource management
func RunCommand(name string, args []string, config *CommandConfig) ([]byte, error) {
    if config == nil {
        config = DefaultCommandConfig()
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, name, args...)
    
    // Set up pipes before starting
    output, err := cmd.Output()
    
    if ctx.Err() == context.DeadlineExceeded {
        return nil, fmt.Errorf("command %s timed out after %v", name, config.Timeout)
    }
    
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            // Special handling for known exit codes
            if exitErr.ExitCode() == 1 && len(output) == 0 {
                // Empty clipboard on some systems
                return []byte{}, nil
            }
            return nil, fmt.Errorf("command %s failed with exit code %d: %w", 
                name, exitErr.ExitCode(), err)
        }
        return nil, fmt.Errorf("command %s failed: %w", name, err)
    }
    
    if len(output) > config.MaxOutputSize {
        return nil, fmt.Errorf("command output exceeds maximum size of %d bytes", 
            config.MaxOutputSize)
    }
    
    return output, nil
}

// RunCommandWithInput executes a command with stdin input
func RunCommandWithInput(name string, args []string, input []byte, config *CommandConfig) error {
    if config == nil {
        config = DefaultCommandConfig()
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, name, args...)
    
    // Set up stdin pipe
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return fmt.Errorf("failed to create stdin pipe: %w", err)
    }
    
    // Ensure cleanup happens regardless of error
    defer func() {
        if stdin != nil {
            _ = stdin.Close()
        }
        if cmd.Process != nil {
            _ = cmd.Process.Kill()
        }
    }()
    
    // Start command
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("failed to start %s: %w", name, err)
    }
    
    // Write input
    if _, err := stdin.Write(input); err != nil {
        return fmt.Errorf("failed to write to %s: %w", name, err)
    }
    
    // Close stdin to signal EOF
    if err := stdin.Close(); err != nil {
        return fmt.Errorf("failed to close stdin: %w", err)
    }
    stdin = nil // Prevent double close in defer
    
    // Wait for completion
    if err := cmd.Wait(); err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return fmt.Errorf("command %s timed out after %v", name, config.Timeout)
        }
        return fmt.Errorf("%s failed: %w", name, err)
    }
    
    return nil
}
```

#### 2. Size Limits and Validation

```go
// clipboard/limits.go
package clipboard

import (
    "fmt"
    "unicode/utf8"
)

const (
    // MaxClipboardSize is the maximum allowed clipboard content size (10MB)
    MaxClipboardSize = 10 * 1024 * 1024
    
    // MaxReasonableSize for normal text content (1MB)
    MaxReasonableSize = 1024 * 1024
)

// ValidateContent checks if clipboard content is within acceptable limits
func ValidateContent(content []byte) error {
    if len(content) > MaxClipboardSize {
        return fmt.Errorf("clipboard content too large: %d bytes (max: %d)", 
            len(content), MaxClipboardSize)
    }
    
    // Warn for large but valid content
    if len(content) > MaxReasonableSize {
        // This would use the logger from CommandConfig
        // Log warning about large clipboard content
    }
    
    // Ensure valid UTF-8 for text operations
    if !utf8.Valid(content) {
        return fmt.Errorf("clipboard content contains invalid UTF-8")
    }
    
    return nil
}
```

#### 3. Platform Implementation Updates

```go
// Updated darwin.go Read method
func (c *DarwinClipboard) Read() (string, error) {
    output, err := RunCommand("pbpaste", nil, c.cmdConfig)
    if err != nil {
        return "", fmt.Errorf("clipboard read failed: %w", err)
    }
    
    if err := ValidateContent(output); err != nil {
        return "", err
    }
    
    return string(output), nil
}

// Updated darwin.go Write method  
func (c *DarwinClipboard) Write(content string) error {
    contentBytes := []byte(content)
    if err := ValidateContent(contentBytes); err != nil {
        return err
    }
    
    if err := RunCommandWithInput("pbcopy", nil, contentBytes, c.cmdConfig); err != nil {
        return fmt.Errorf("clipboard write failed: %w", err)
    }
    
    // Update tracking state only after successful write
    c.mu.Lock()
    c.lastHash = c.hashContent(content)
    c.lastChangeCount = c.getChangeCount()
    c.sequenceNumber.Add(1)
    c.lastModified = time.Now()
    c.mu.Unlock()
    
    return nil
}
```

### Testing Strategy

```go
// clipboard/command_test.go
func TestCommandTimeout(t *testing.T) {
    config := &CommandConfig{
        Timeout: 100 * time.Millisecond,
    }
    
    // This should timeout
    _, err := RunCommand("sleep", []string{"1"}, config)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timed out")
}

func TestContentValidation(t *testing.T) {
    tests := []struct {
        name    string
        content []byte
        wantErr bool
    }{
        {"valid small", []byte("hello"), false},
        {"valid large", make([]byte, MaxReasonableSize), false},
        {"too large", make([]byte, MaxClipboardSize+1), true},
        {"invalid utf8", []byte{0xff, 0xfe, 0xfd}, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateContent(tt.content)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}

func TestResourceCleanup(t *testing.T) {
    // Test that resources are cleaned up even on panic
    config := &CommandConfig{Timeout: 5 * time.Second}
    
    // Force an error condition
    _, err := RunCommandWithInput("false", nil, []byte("test"), config)
    assert.Error(t, err)
    
    // Verify no resource leaks (this would be more comprehensive in real tests)
}
```

This production hardening approach focuses on the critical issues that could affect reliability in real-world usage, while maintaining the simplicity and elegance of the existing architecture.
