package mesh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Veraticus/linkpearl/pkg/transport"
)

// Type alias to avoid shadowing in test functions
type transportConn = transport.Conn

// mockTopologyConn implements transport.Conn for testing
type mockTopologyConn struct {
	nodeID      string
	mode        string
	version     string
	sendChan    chan interface{}
	receiveChan chan interface{}
	closed      atomic.Bool
	sendErr     error
	receiveErr  error
	mu          sync.Mutex
}

func newMockTopologyConn(nodeID, mode string) *mockTopologyConn {
	return &mockTopologyConn{
		nodeID:      nodeID,
		mode:        mode,
		sendChan:    make(chan interface{}, 100),
		receiveChan: make(chan interface{}, 100),
	}
}

func (c *mockTopologyConn) NodeID() string  { return c.nodeID }
func (c *mockTopologyConn) Mode() string    { return c.mode }
func (c *mockTopologyConn) Version() string { return c.version }

func (c *mockTopologyConn) Send(msg interface{}) error {
	if c.closed.Load() {
		return errors.New("connection closed")
	}
	c.mu.Lock()
	err := c.sendErr
	c.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case c.sendChan <- msg:
		return nil
	case <-time.After(10 * time.Millisecond):
		return errors.New("send timeout")
	}
}

func (c *mockTopologyConn) Receive(msg interface{}) error {
	if c.closed.Load() {
		return errors.New("connection closed")
	}
	c.mu.Lock()
	err := c.receiveErr
	c.mu.Unlock()
	if err != nil {
		return err
	}
	
	// Create a done channel to detect when connection is closed
	done := make(chan struct{})
	go func() {
		// Poll for closed state
		for !c.closed.Load() {
			time.Sleep(10 * time.Millisecond)
		}
		close(done)
	}()
	
	select {
	case data := <-c.receiveChan:
		// Handle different data types
		switch v := data.(type) {
		case []byte:
			// If it's already bytes, unmarshal directly
			return json.Unmarshal(v, msg)
		case string:
			// If it's a string, convert to bytes and unmarshal
			return json.Unmarshal([]byte(v), msg)
		default:
			// For other types, marshal then unmarshal
			jsonData, err := json.Marshal(data)
			if err != nil {
				return err
			}
			return json.Unmarshal(jsonData, msg)
		}
	case <-done:
		return errors.New("connection closed")
	}
}

func (c *mockTopologyConn) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}
	// Don't close channels - let them be garbage collected
	// Closing would cause panics if someone tries to send
	return nil
}

func (c *mockTopologyConn) SetSendError(err error) {
	c.mu.Lock()
	c.sendErr = err
	c.mu.Unlock()
}

func (c *mockTopologyConn) SetReceiveError(err error) {
	c.mu.Lock()
	c.receiveErr = err
	c.mu.Unlock()
}

// Implement remaining Conn interface methods
func (c *mockTopologyConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
}

func (c *mockTopologyConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8081}
}

func (c *mockTopologyConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *mockTopologyConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *mockTopologyConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// mockTopologyTransport implements transport.Transport for testing
type mockTopologyTransport struct {
	addr         string
	listening    atomic.Bool
	acceptChan   chan transport.Conn
	connectFunc  func(ctx context.Context, addr string) (transportConn, error)
	connections  map[string]*mockTopologyConn
	mu           sync.RWMutex
	acceptErr    error
	listenErr    error
	closed       atomic.Bool
}

func newMockTopologyTransport() *mockTopologyTransport {
	return &mockTopologyTransport{
		acceptChan:  make(chan transport.Conn, 10),
		connections: make(map[string]*mockTopologyConn),
	}
}

func (t *mockTopologyTransport) Listen(addr string) error {
	if t.listenErr != nil {
		return t.listenErr
	}
	t.addr = addr
	t.listening.Store(true)
	return nil
}

func (t *mockTopologyTransport) Accept() (transport.Conn, error) {
	if t.closed.Load() || !t.listening.Load() {
		return nil, errors.New("transport closed")
	}
	if t.acceptErr != nil {
		return nil, t.acceptErr
	}
	select {
	case conn, ok := <-t.acceptChan:
		if !ok {
			return nil, errors.New("transport closed")
		}
		return conn, nil
	case <-time.After(100 * time.Millisecond):
		if t.closed.Load() {
			return nil, errors.New("transport closed")
		}
		return nil, errors.New("accept timeout")
	}
}

func (t *mockTopologyTransport) Connect(ctx context.Context, addr string) (transport.Conn, error) {
	if t.connectFunc != nil {
		return t.connectFunc(ctx, addr)
	}
	// Default implementation creates a mock connection
	conn := newMockTopologyConn(fmt.Sprintf("node-%s", addr), "full")
	t.mu.Lock()
	t.connections[addr] = conn
	t.mu.Unlock()
	return conn, nil
}

func (t *mockTopologyTransport) Close() error {
	t.closed.Store(true)
	t.listening.Store(false)
	if t.acceptChan != nil {
		close(t.acceptChan)
	}
	t.mu.Lock()
	for _, conn := range t.connections {
		_ = conn.Close()
	}
	t.connections = make(map[string]*mockTopologyConn)
	t.mu.Unlock()
	return nil
}

func (t *mockTopologyTransport) AddIncomingConnection(conn transport.Conn) {
	if t.closed.Load() {
		return
	}
	select {
	case t.acceptChan <- conn:
	default:
		// Channel full or closed
	}
}

func (t *mockTopologyTransport) Addr() net.Addr {
	if !t.listening.Load() {
		return nil
	}
	// Parse the address string to create a proper net.Addr
	host, port, _ := net.SplitHostPort(t.addr)
	if host == "" {
		host = "127.0.0.1"
	}
	tcpAddr, _ := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, port))
	return tcpAddr
}

// testLogger implements Logger for testing
type testLogger struct {
	t      *testing.T
	prefix string
}

func newTestLogger(t *testing.T, prefix string) *testLogger {
	return &testLogger{t: t, prefix: prefix}
}

func (l *testLogger) Debug(msg string, args ...interface{}) {
	l.log("DEBUG", msg, args...)
}

func (l *testLogger) Info(msg string, args ...interface{}) {
	l.log("INFO", msg, args...)
}

func (l *testLogger) Warn(msg string, args ...interface{}) {
	l.log("WARN", msg, args...)
}

func (l *testLogger) Error(msg string, args ...interface{}) {
	l.log("ERROR", msg, args...)
}

func (l *testLogger) log(level, msg string, args ...interface{}) {
	// Format args as key=value pairs
	var kvPairs []string
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			kvPairs = append(kvPairs, fmt.Sprintf("%v=%v", args[i], args[i+1]))
		}
	}
	l.t.Logf("[%s] %s: %s %v", l.prefix, level, msg, kvPairs)
}

// Test topology creation
func TestNewTopology(t *testing.T) {
	tests := []struct {
		name    string
		config  *TopologyConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config is required",
		},
		{
			name: "invalid node - no ID",
			config: &TopologyConfig{
				Self:      Node{Mode: "full", Addr: "localhost:8080"},
				Transport: newMockTopologyTransport(),
			},
			wantErr: true,
			errMsg:  "node ID is required",
		},
		{
			name: "invalid node - bad mode",
			config: &TopologyConfig{
				Self:      Node{ID: "node1", Mode: "invalid"},
				Transport: newMockTopologyTransport(),
			},
			wantErr: true,
			errMsg:  "node mode must be",
		},
		{
			name: "full node without address",
			config: &TopologyConfig{
				Self:      Node{ID: "node1", Mode: "full"},
				Transport: newMockTopologyTransport(),
			},
			wantErr: true,
			errMsg:  "full nodes must have a listen address",
		},
		{
			name: "client node with address",
			config: &TopologyConfig{
				Self:      Node{ID: "node1", Mode: "client", Addr: "localhost:8080"},
				Transport: newMockTopologyTransport(),
			},
			wantErr: true,
			errMsg:  "client nodes should not have a listen address",
		},
		{
			name: "valid full node",
			config: &TopologyConfig{
				Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
				Transport: newMockTopologyTransport(),
				JoinAddrs: []string{"localhost:8081", "localhost:8082"},
			},
			wantErr: false,
		},
		{
			name: "valid client node",
			config: &TopologyConfig{
				Self:      Node{ID: "node1", Mode: "client"},
				Transport: newMockTopologyTransport(),
				JoinAddrs: []string{"localhost:8081"},
			},
			wantErr: false,
		},
		{
			name: "with custom parameters",
			config: &TopologyConfig{
				Self:                 Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
				Transport:            newMockTopologyTransport(),
				ReconnectInterval:    5 * time.Second,
				MaxReconnectInterval: 10 * time.Minute,
				EventBufferSize:      2000,
				MessageBufferSize:    2000,
				Logger:               newTestLogger(t, "test"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo, err := NewTopology(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, topo)
				
				// Verify internal state
				impl := topo.(*topology)
				assert.Equal(t, tt.config.Self, impl.self)
				assert.Equal(t, tt.config.Transport, impl.transport)
				assert.NotNil(t, impl.peers)
				assert.NotNil(t, impl.backoffs)
				assert.NotNil(t, impl.events)
				assert.NotNil(t, impl.messages)
				assert.NotNil(t, impl.router)
				
				// Clean up
				_ = topo.Stop()
			}
		})
	}
}

// Test start/stop lifecycle
func TestTopologyLifecycle(t *testing.T) {
	t.Run("full node start and stop", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Verify transport is listening
		assert.True(t, transport.listening.Load())
		assert.Equal(t, "localhost:8080", transport.addr)

		// Starting again should fail
		err = topo.Start(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already started")

		// Stop topology
		err = topo.Stop()
		require.NoError(t, err)

		// Verify transport is closed
		assert.False(t, transport.listening.Load())

		// Stopping again should be idempotent
		err = topo.Stop()
		assert.NoError(t, err)
	})

	t.Run("client node start and stop", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "client1", Mode: "client"},
			Transport: transport,
			Logger:    newTestLogger(t, "client1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Verify transport is NOT listening (client mode)
		assert.False(t, transport.listening.Load())

		// Stop topology
		err = topo.Stop()
		require.NoError(t, err)
	})

	t.Run("start with listen error", func(t *testing.T) {
		transport := newMockTopologyTransport()
		transport.listenErr = errors.New("address in use")
		
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)

		// Start should fail
		err = topo.Start(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to listen")
		
		// Clean up
		_ = topo.Stop()
	})

	t.Run("operations on closed topology", func(t *testing.T) {
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: newMockTopologyTransport(),
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)

		// Stop before starting
		err = topo.Stop()
		require.NoError(t, err)

		// Operations should fail with ErrClosed
		err = topo.Start(context.Background())
		assert.ErrorIs(t, err, ErrClosed)

		err = topo.AddJoinAddr("localhost:8081")
		assert.ErrorIs(t, err, ErrClosed)

		err = topo.RemoveJoinAddr("localhost:8081")
		assert.ErrorIs(t, err, ErrClosed)
	})
}

// Test join address management
func TestJoinAddressManagement(t *testing.T) {
	t.Run("add and remove join addresses", func(t *testing.T) {
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: newMockTopologyTransport(),
			JoinAddrs: []string{"localhost:8081"},
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		impl := topo.(*topology)

		// Initial join addresses
		assert.Len(t, impl.joinAddrs, 1)
		assert.Contains(t, impl.joinAddrs, "localhost:8081")

		// Add new address
		err = topo.AddJoinAddr("localhost:8082")
		require.NoError(t, err)
		assert.Len(t, impl.joinAddrs, 2)
		assert.Contains(t, impl.joinAddrs, "localhost:8082")

		// Add duplicate - should be idempotent
		err = topo.AddJoinAddr("localhost:8082")
		require.NoError(t, err)
		assert.Len(t, impl.joinAddrs, 2)

		// Remove address
		err = topo.RemoveJoinAddr("localhost:8081")
		require.NoError(t, err)
		assert.Len(t, impl.joinAddrs, 1)
		assert.NotContains(t, impl.joinAddrs, "localhost:8081")

		// Remove non-existent - should be idempotent
		err = topo.RemoveJoinAddr("localhost:8083")
		assert.NoError(t, err)
	})

	t.Run("add join address after start", func(t *testing.T) {
		connected := make(chan string, 1)
		transport := newMockTopologyTransport()
		transport.connectFunc = func(ctx context.Context, addr string) (transportConn, error) {
			connected <- addr
			return newMockTopologyConn("remote-node", "full"), nil
		}

		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "client"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add join address - should trigger connection
		err = topo.AddJoinAddr("localhost:8081")
		require.NoError(t, err)

		// Give the goroutine time to start (especially important with race detector)
		time.Sleep(10 * time.Millisecond)

		// Wait for connection attempt
		select {
		case addr := <-connected:
			assert.Equal(t, "localhost:8081", addr)
		case <-time.After(2 * time.Second):
			t.Fatal("connection not attempted")
		}
	})
}

// Test incoming connections
func TestIncomingConnections(t *testing.T) {
	t.Run("accept incoming connection", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Subscribe to events
		events := topo.Events()

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Simulate incoming connection
		incomingConn := newMockTopologyConn("remote-node", "client")
		transport.AddIncomingConnection(incomingConn)

		// Wait for peer connected event
		select {
		case event := <-events:
			assert.Equal(t, PeerConnected, event.Type)
			assert.Equal(t, "remote-node", event.Peer.ID)
			assert.Equal(t, "client", event.Peer.Mode)
		case <-time.After(time.Second):
			t.Fatal("peer connected event not received")
		}

		// Check peer info
		peerInfo, err := topo.GetPeer("remote-node")
		require.NoError(t, err)
		assert.Equal(t, "remote-node", peerInfo.ID)
		assert.Equal(t, "inbound", peerInfo.Direction)
		assert.Equal(t, 1, topo.PeerCount())
	})

	t.Run("accept multiple connections", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Accept multiple connections
		for i := 0; i < 3; i++ {
			conn := newMockTopologyConn(fmt.Sprintf("node-%d", i), "full")
			transport.AddIncomingConnection(conn)
		}

		// Wait for connections
		time.Sleep(200 * time.Millisecond)

		// Verify all peers connected
		assert.Equal(t, 3, topo.PeerCount())
		peers := topo.GetPeers()
		assert.Len(t, peers, 3)
	})

	t.Run("reject duplicate peer", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// First connection
		conn1 := newMockTopologyConn("duplicate-node", "full")
		transport.AddIncomingConnection(conn1)

		// Wait for first connection
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, 1, topo.PeerCount())

		// Second connection with same ID should be rejected
		conn2 := newMockTopologyConn("duplicate-node", "full")
		transport.AddIncomingConnection(conn2)

		// Wait and verify still only one peer
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, 1, topo.PeerCount())

		// Second connection should be closed
		assert.Eventually(t, func() bool {
			return conn2.closed.Load()
		}, time.Second, 10*time.Millisecond, "connection should be closed")
	})
}

// Test outbound connections with backoff
func TestOutboundConnections(t *testing.T) {
	t.Run("successful connection", func(t *testing.T) {
		connected := make(chan bool, 1)
		transport := newMockTopologyTransport()
		transport.connectFunc = func(ctx context.Context, addr string) (transportConn, error) {
			connected <- true
			conn := newMockTopologyConn("remote-node", "full")
			conn.version = "localhost:9090" // Remote listen address
			return conn, nil
		}

		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "client"},
			Transport: transport,
			JoinAddrs: []string{"localhost:8081"},
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Subscribe to events
		events := topo.Events()

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Wait for connection
		select {
		case <-connected:
		case <-time.After(time.Second):
			t.Fatal("connection not attempted")
		}

		// Wait for peer connected event
		select {
		case event := <-events:
			assert.Equal(t, PeerConnected, event.Type)
			assert.Equal(t, "remote-node", event.Peer.ID)
		case <-time.After(time.Second):
			t.Fatal("peer connected event not received")
		}

		// Verify peer info
		peerInfo, err := topo.GetPeer("remote-node")
		require.NoError(t, err)
		assert.Equal(t, "outbound", peerInfo.Direction)
		assert.Equal(t, "localhost:9090", peerInfo.Addr)
	})

	t.Run("connection with retry and backoff", func(t *testing.T) {
		attempts := atomic.Int32{}
		transport := newMockTopologyTransport()
		transport.connectFunc = func(ctx context.Context, addr string) (transportConn, error) {
			count := attempts.Add(1)
			if count < 3 {
				return nil, errors.New("connection refused")
			}
			return newMockTopologyConn("remote-node", "full"), nil
		}

		config := &TopologyConfig{
			Self:              Node{ID: "node1", Mode: "client"},
			Transport:         transport,
			JoinAddrs:         []string{"localhost:8081"},
			ReconnectInterval: 50 * time.Millisecond,
			Logger:            newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Wait for successful connection
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if topo.PeerCount() > 0 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		// Verify connection succeeded after retries
		assert.Equal(t, 1, topo.PeerCount())
		assert.GreaterOrEqual(t, attempts.Load(), int32(3))
	})

	t.Run("avoid duplicate outbound connections", func(t *testing.T) {
		transport := newMockTopologyTransport()
		transport.connectFunc = func(ctx context.Context, addr string) (transportConn, error) {
			// Always return the same node ID
			return newMockTopologyConn("same-node", "full"), nil
		}

		config := &TopologyConfig{
			Self:              Node{ID: "node1", Mode: "client"},
			Transport:         transport,
			JoinAddrs:         []string{"localhost:8081", "localhost:8082"},
			ReconnectInterval: 50 * time.Millisecond,
			Logger:            newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Start topology
		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Wait a bit for connection attempts
		time.Sleep(200 * time.Millisecond)

		// Should only have one peer despite two addresses
		assert.Equal(t, 1, topo.PeerCount())
	})
}

// Test message routing and handling
func TestMessageRouting(t *testing.T) {
	t.Run("send to specific peer", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add incoming peer
		peerConn := newMockTopologyConn("peer1", "full")
		transport.AddIncomingConnection(peerConn)

		// Wait for connection
		time.Sleep(100 * time.Millisecond)

		// Send message to peer
		testMsg := map[string]string{"type": "test", "data": "hello"}
		err = topo.SendToPeer("peer1", testMsg)
		require.NoError(t, err)

		// Verify message sent
		select {
		case msg := <-peerConn.sendChan:
			// Message should be wrapped in mesh message format
			data, ok := msg.([]byte)
			require.True(t, ok)
			
			var meshMsg meshMessage
			err = json.Unmarshal(data, &meshMsg)
			require.NoError(t, err)
			assert.Equal(t, "node1", meshMsg.From)
		case <-time.After(time.Second):
			t.Fatal("message not sent")
		}
	})

	t.Run("broadcast to all peers", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add multiple peers
		var peers []*mockTopologyConn
		for i := 0; i < 3; i++ {
			conn := newMockTopologyConn(fmt.Sprintf("peer%d", i), "full")
			peers = append(peers, conn)
			transport.AddIncomingConnection(conn)
		}

		// Wait for connections
		time.Sleep(200 * time.Millisecond)
		assert.Equal(t, 3, topo.PeerCount())

		// Broadcast message
		testMsg := map[string]string{"type": "broadcast", "data": "hello all"}
		err = topo.Broadcast(testMsg)
		require.NoError(t, err)

		// Verify all peers received message
		for _, peer := range peers {
			select {
			case msg := <-peer.sendChan:
				require.NotNil(t, msg)
			case <-time.After(time.Second):
				t.Fatalf("peer %s did not receive broadcast", peer.nodeID)
			}
		}
	})

	t.Run("receive and route messages", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Subscribe to messages
		messages := topo.Messages()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add peer
		peerConn := newMockTopologyConn("peer1", "full")
		transport.AddIncomingConnection(peerConn)
		time.Sleep(100 * time.Millisecond)

		// Send clipboard message from peer
		clipboardMsg := map[string]string{"content": "test clipboard data"}
		msgData, err := marshalMessage(MessageTypeClipboard, "peer1", clipboardMsg)
		require.NoError(t, err)
		
		select {
		case peerConn.receiveChan <- msgData:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("failed to send message to peer")
		}

		// Should receive the message
		select {
		case msg := <-messages:
			assert.Equal(t, "peer1", msg.From)
			assert.Equal(t, string(MessageTypeClipboard), msg.Type)
		case <-time.After(time.Second):
			t.Fatal("message not received")
		}
	})

	t.Run("handle ping-pong messages", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add peer
		peerConn := newMockTopologyConn("peer1", "full")
		transport.AddIncomingConnection(peerConn)
		time.Sleep(100 * time.Millisecond)

		// Drain the initial peer list message
		select {
		case <-peerConn.sendChan:
			// Expected - peer list is sent on connection
		case <-time.After(100 * time.Millisecond):
			t.Fatal("peer list not received")
		}

		// Send ping from peer
		pingMsg := PingMessage{Timestamp: time.Now().Unix()}
		msgData, err := marshalMessage(MessageTypePing, "peer1", pingMsg)
		require.NoError(t, err)
		
		select {
		case peerConn.receiveChan <- msgData:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("failed to send message to peer")
		}

		// Should receive pong response
		select {
		case msg := <-peerConn.sendChan:
			data, ok := msg.([]byte)
			require.True(t, ok)
			
			msgType, from, payload, err := unmarshalMessage(data)
			require.NoError(t, err)
			assert.Equal(t, MessageTypePong, msgType)
			assert.Equal(t, "node1", from)
			
			var pong PongMessage
			err = json.Unmarshal(payload, &pong)
			require.NoError(t, err)
			assert.Equal(t, pingMsg.Timestamp, pong.Timestamp)
		case <-time.After(time.Second):
			t.Fatal("pong not received")
		}
	})
}

// Test peer list exchange
func TestPeerListExchange(t *testing.T) {
	transport := newMockTopologyTransport()
	config := &TopologyConfig{
		Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
		Transport: transport,
		Logger:    newTestLogger(t, "node1"),
	}

	topo, err := NewTopology(config)
	require.NoError(t, err)
	defer func() { _ = topo.Stop() }()

	err = topo.Start(context.Background())
	require.NoError(t, err)

	// Add first peer
	peer1Conn := newMockTopologyConn("peer1", "full")
	transport.AddIncomingConnection(peer1Conn)
	
	// Should receive peer list
	select {
	case msg := <-peer1Conn.sendChan:
		data, ok := msg.([]byte)
		require.True(t, ok)
		
		msgType, from, payload, err := unmarshalMessage(data)
		require.NoError(t, err)
		assert.Equal(t, MessageTypePeerList, msgType)
		assert.Equal(t, "node1", from)
		
		var peerList PeerListMessage
		err = json.Unmarshal(payload, &peerList)
		require.NoError(t, err)
		assert.Len(t, peerList.Peers, 1) // Only self
		assert.Equal(t, "node1", peerList.Peers[0].ID)
	case <-time.After(time.Second):
		t.Fatal("peer list not received")
	}

	// Add second peer
	peer2Conn := newMockTopologyConn("peer2", "client")
	transport.AddIncomingConnection(peer2Conn)
	
	// Second peer should receive list with node1 and peer1
	select {
	case msg := <-peer2Conn.sendChan:
		data, ok := msg.([]byte)
		require.True(t, ok)
		
		_, _, payload, err := unmarshalMessage(data)
		require.NoError(t, err)
		
		var peerList PeerListMessage
		err = json.Unmarshal(payload, &peerList)
		require.NoError(t, err)
		assert.Len(t, peerList.Peers, 2) // self and peer1
		
		nodeIDs := make([]string, len(peerList.Peers))
		for i, node := range peerList.Peers {
			nodeIDs[i] = node.ID
		}
		assert.Contains(t, nodeIDs, "node1")
		assert.Contains(t, nodeIDs, "peer1")
		assert.NotContains(t, nodeIDs, "peer2") // Don't include recipient
	case <-time.After(time.Second):
		t.Fatal("peer list not received by peer2")
	}
}

// Test concurrent operations
func TestTopologyConcurrentOperations(t *testing.T) {
	t.Run("concurrent sends", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add peer
		peerConn := newMockTopologyConn("peer1", "full")
		transport.AddIncomingConnection(peerConn)
		time.Sleep(100 * time.Millisecond)

		// Send messages concurrently
		var wg sync.WaitGroup
		errors := make(chan error, 10)
		
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				msg := map[string]int{"id": n}
				if err := topo.SendToPeer("peer1", msg); err != nil {
					errors <- err
				}
			}(i)
		}
		
		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("concurrent send error: %v", err)
		}

		// Verify all messages sent
		received := 0
		timeout := time.After(time.Second)
		for received < 10 {
			select {
			case <-peerConn.sendChan:
				received++
			case <-timeout:
				t.Fatalf("only received %d/10 messages", received)
			}
		}
	})

	t.Run("concurrent peer operations", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add peers concurrently
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				conn := newMockTopologyConn(fmt.Sprintf("peer%d", n), "full")
				transport.AddIncomingConnection(conn)
			}(i)
		}
		
		wg.Wait()
		time.Sleep(200 * time.Millisecond)

		// Verify all peers connected
		assert.Equal(t, 5, topo.PeerCount())

		// Query peers concurrently
		wg = sync.WaitGroup{}
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				
				// Random operations
				switch i % 3 {
				case 0:
					topo.GetPeers()
				case 1:
					topo.PeerCount()
				case 2:
					_, _ = topo.GetPeer(fmt.Sprintf("peer%d", i%5))
				}
			}()
		}
		
		wg.Wait()
	})

	t.Run("concurrent join address modifications", func(t *testing.T) {
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: newMockTopologyTransport(),
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Concurrent adds and removes
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				addr := fmt.Sprintf("localhost:%d", 9000+n)
				
				// Add
				if err := topo.AddJoinAddr(addr); err != nil {
					t.Errorf("add error: %v", err)
				}
				
				// Remove half of them
				if n%2 == 0 {
					if err := topo.RemoveJoinAddr(addr); err != nil {
						t.Errorf("remove error: %v", err)
					}
				}
			}(i)
		}
		
		wg.Wait()

		// Verify state is consistent
		impl := topo.(*topology)
		impl.mu.RLock()
		joinCount := len(impl.joinAddrs)
		impl.mu.RUnlock()
		
		// Should have 5 addresses (odd numbers)
		assert.Equal(t, 5, joinCount)
	})
}

// Test error handling
func TestErrorHandling(t *testing.T) {
	t.Run("peer connection errors", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add peer with receive error
		peerConn := newMockTopologyConn("peer1", "full")
		peerConn.SetReceiveError(errors.New("connection reset"))
		transport.AddIncomingConnection(peerConn)

		// Subscribe to events
		events := topo.Events()

		// Wait for disconnect event
		select {
		case event := <-events:
			if event.Type == PeerConnected {
				// Wait for disconnect
				select {
				case event = <-events:
					assert.Equal(t, PeerDisconnected, event.Type)
					assert.Equal(t, "peer1", event.Peer.ID)
				case <-time.After(time.Second):
					t.Fatal("disconnect event not received")
				}
			} else {
				assert.Equal(t, PeerDisconnected, event.Type)
			}
		case <-time.After(time.Second):
			t.Fatal("no events received")
		}

		// Peer should be removed
		_, err = topo.GetPeer("peer1")
		assert.ErrorIs(t, err, ErrPeerNotFound)
	})

	t.Run("send to non-existent peer", func(t *testing.T) {
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: newMockTopologyTransport(),
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Send to non-existent peer
		err = topo.SendToPeer("ghost", map[string]string{"test": "data"})
		assert.ErrorIs(t, err, ErrPeerNotFound)
	})

	t.Run("message buffer overflow", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:              Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport:         transport,
			MessageBufferSize: 2, // Very small buffer
			Logger:            newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add peer
		peerConn := newMockTopologyConn("peer1", "full")
		transport.AddIncomingConnection(peerConn)
		time.Sleep(100 * time.Millisecond)

		// Send many messages quickly
		for i := 0; i < 10; i++ {
			msg := map[string]int{"seq": i}
			msgData, _ := marshalMessage(MessageTypeClipboard, "peer1", msg)
			select {
			case peerConn.receiveChan <- msgData:
			case <-time.After(10 * time.Millisecond):
				// Connection might be closed, exit the loop
				goto done
			}
		}
		done:

		// Some messages should be dropped due to buffer overflow
		// Just verify no panic
		time.Sleep(100 * time.Millisecond)
	})
}

// Test event system
func TestEventSystem(t *testing.T) {
	t.Run("multiple event subscribers", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport: transport,
			Logger:    newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Create multiple subscribers
		subs := make([]<-chan TopologyEvent, 3)
		for i := 0; i < 3; i++ {
			subs[i] = topo.Events()
		}

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Add peer
		peerConn := newMockTopologyConn("peer1", "full")
		transport.AddIncomingConnection(peerConn)

		// All subscribers should receive event
		for i, sub := range subs {
			select {
			case event := <-sub:
				assert.Equal(t, PeerConnected, event.Type)
				assert.Equal(t, "peer1", event.Peer.ID)
			case <-time.After(time.Second):
				t.Fatalf("subscriber %d did not receive event", i)
			}
		}
	})

	t.Run("event buffer and ordering", func(t *testing.T) {
		transport := newMockTopologyTransport()
		config := &TopologyConfig{
			Self:            Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
			Transport:       transport,
			EventBufferSize: 10,
			Logger:          newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Subscribe before creating events to test buffering
		events := topo.Events()

		// Add and remove peers quickly
		var peers []*mockTopologyConn
		for i := 0; i < 5; i++ {
			conn := newMockTopologyConn(fmt.Sprintf("peer%d", i), "full")
			peers = append(peers, conn)
			transport.AddIncomingConnection(conn)
		}

		// Wait for connections
		time.Sleep(200 * time.Millisecond)

		// Close all connections
		for _, conn := range peers {
			_ = conn.Close()
		}

		// Wait a bit for disconnects to process
		time.Sleep(100 * time.Millisecond)

		// Should still receive buffered events
		connectCount := 0
		disconnectCount := 0
		
		timeout := time.After(time.Second)
		for connectCount < 5 || disconnectCount < 5 {
			select {
			case event := <-events:
				switch event.Type {
				case PeerConnected:
					connectCount++
				case PeerDisconnected:
					disconnectCount++
				}
			case <-timeout:
				t.Fatalf("got %d connects and %d disconnects, expected 5 each", 
					connectCount, disconnectCount)
			}
		}
	})
}

// Test reconnection behavior
func TestReconnectionBehavior(t *testing.T) {
	t.Run("reconnect after disconnect", func(t *testing.T) {
		connectCount := atomic.Int32{}
		transport := newMockTopologyTransport()
		
		// Track connections
		var currentConn *mockTopologyConn
		var mu sync.Mutex
		
		transport.connectFunc = func(ctx context.Context, addr string) (transportConn, error) {
			count := connectCount.Add(1)
			
			mu.Lock()
			defer mu.Unlock()
			
			switch count {
			case 1:
				// First connection
				currentConn = newMockTopologyConn("remote-node", "full")
				return currentConn, nil
			case 2:
				// Fail second attempt
				return nil, errors.New("connection refused")
			default:
				// Third attempt succeeds
				currentConn = newMockTopologyConn("remote-node", "full")
				return currentConn, nil
			}
		}

		config := &TopologyConfig{
			Self:              Node{ID: "node1", Mode: "client"},
			Transport:         transport,
			JoinAddrs:         []string{"localhost:8081"},
			ReconnectInterval: 50 * time.Millisecond,
			Logger:            newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)
		defer func() { _ = topo.Stop() }()

		// Subscribe to events
		events := topo.Events()

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Wait for first connection
		select {
		case event := <-events:
			assert.Equal(t, PeerConnected, event.Type)
		case <-time.After(time.Second):
			t.Fatal("first connection not established")
		}

		// Simulate disconnect
		mu.Lock()
		if currentConn != nil {
			_ = currentConn.Close()
		}
		mu.Unlock()

		// Wait for disconnect event
		select {
		case event := <-events:
			assert.Equal(t, PeerDisconnected, event.Type)
		case <-time.After(time.Second):
			t.Fatal("disconnect event not received")
		}

		// Wait for reconnection
		select {
		case event := <-events:
			assert.Equal(t, PeerConnected, event.Type)
		case <-time.After(2 * time.Second):
			t.Fatal("reconnection did not occur")
		}

		// Verify at least 3 connection attempts
		assert.GreaterOrEqual(t, connectCount.Load(), int32(3))
	})

	t.Run("exponential backoff", func(t *testing.T) {
		attempts := make([]time.Time, 0)
		var mu sync.Mutex
		
		transport := newMockTopologyTransport()
		transport.connectFunc = func(ctx context.Context, addr string) (transportConn, error) {
			mu.Lock()
			attempts = append(attempts, time.Now())
			mu.Unlock()
			
			// Always fail to see backoff pattern
			return nil, errors.New("connection refused")
		}

		config := &TopologyConfig{
			Self:                 Node{ID: "node1", Mode: "client"},
			Transport:            transport,
			JoinAddrs:            []string{"localhost:8081"},
			ReconnectInterval:    50 * time.Millisecond,
			MaxReconnectInterval: 400 * time.Millisecond,
			Logger:               newTestLogger(t, "node1"),
		}

		topo, err := NewTopology(config)
		require.NoError(t, err)

		err = topo.Start(context.Background())
		require.NoError(t, err)

		// Let it try a few times
		time.Sleep(time.Second)
		
		// Stop to end the test
		_ = topo.Stop()

		// Verify backoff pattern
		mu.Lock()
		defer mu.Unlock()
		
		require.GreaterOrEqual(t, len(attempts), 3)
		
		// Check intervals are increasing
		for i := 2; i < len(attempts); i++ {
			interval1 := attempts[i-1].Sub(attempts[i-2])
			interval2 := attempts[i].Sub(attempts[i-1])
			
			// Later interval should be >= earlier (with some tolerance)
			assert.GreaterOrEqual(t, interval2.Milliseconds(), 
				interval1.Milliseconds()-10) // 10ms tolerance
		}
	})
}

// Benchmark tests
func BenchmarkTopologyBroadcast(b *testing.B) {
	transport := newMockTopologyTransport()
	config := &TopologyConfig{
		Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
		Transport: transport,
		Logger:    noopLogger{},
	}

	topo, err := NewTopology(config)
	require.NoError(b, err)
	defer func() { _ = topo.Stop() }()

	err = topo.Start(context.Background())
	require.NoError(b, err)

	// Add peers
	for i := 0; i < 10; i++ {
		conn := newMockTopologyConn(fmt.Sprintf("peer%d", i), "full")
		transport.AddIncomingConnection(conn)
	}
	time.Sleep(200 * time.Millisecond)

	msg := map[string]string{"type": "benchmark", "data": "test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = topo.Broadcast(msg)
	}
}

func BenchmarkTopologyConcurrentSends(b *testing.B) {
	transport := newMockTopologyTransport()
	config := &TopologyConfig{
		Self:      Node{ID: "node1", Mode: "full", Addr: "localhost:8080"},
		Transport: transport,
		Logger:    noopLogger{},
	}

	topo, err := NewTopology(config)
	require.NoError(b, err)
	defer func() { _ = topo.Stop() }()

	err = topo.Start(context.Background())
	require.NoError(b, err)

	// Add peer
	conn := newMockTopologyConn("peer1", "full")
	transport.AddIncomingConnection(conn)
	time.Sleep(100 * time.Millisecond)

	msg := map[string]string{"type": "benchmark", "data": "test"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = topo.SendToPeer("peer1", msg)
		}
	})
}