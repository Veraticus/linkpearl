package mesh

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Veraticus/linkpearl/pkg/transport"
	"github.com/stretchr/testify/assert"
)

// mockConn implements transport.Conn for testing.
type mockConn struct {
	nodeID       string
	mode         string
	version      string
	messages     chan interface{}
	closed       atomic.Bool
	closeChan    chan struct{}
	sendErr      error
	receiveErr   error
	closeMutex   sync.Mutex
	blockSend    bool
	blockReceive bool
	mu           sync.RWMutex
}

func newMockConn(nodeID, mode string) *mockConn {
	return &mockConn{
		nodeID:    nodeID,
		mode:      mode,
		version:   "1.0.0",
		messages:  make(chan interface{}, 100),
		closeChan: make(chan struct{}),
	}
}

func (c *mockConn) NodeID() string  { return c.nodeID }
func (c *mockConn) Mode() string    { return c.mode }
func (c *mockConn) Version() string { return c.version }

func (c *mockConn) Send(msg interface{}) error {
	if c.closed.Load() {
		return transport.ErrClosed
	}

	c.mu.RLock()
	if c.sendErr != nil {
		err := c.sendErr
		c.mu.RUnlock()
		return err
	}
	if c.blockSend {
		c.mu.RUnlock()
		<-c.closeChan // Block until closed
		return transport.ErrClosed
	}
	c.mu.RUnlock()

	select {
	case c.messages <- msg:
		return nil
	case <-c.closeChan:
		return transport.ErrClosed
	default:
		return errors.New("buffer full")
	}
}

func (c *mockConn) Receive(msg interface{}) error {
	if c.closed.Load() {
		return transport.ErrClosed
	}

	c.mu.RLock()
	if c.receiveErr != nil {
		err := c.receiveErr
		c.mu.RUnlock()
		return err
	}
	if c.blockReceive {
		c.mu.RUnlock()
		<-c.closeChan // Block until closed
		return transport.ErrClosed
	}
	c.mu.RUnlock()

	select {
	case received := <-c.messages:
		if v, ok := msg.(*interface{}); ok {
			*v = received
		}
		return nil
	case <-c.closeChan:
		return transport.ErrClosed
	case <-time.After(10 * time.Millisecond):
		return errors.New("no data available")
	}
}

func (c *mockConn) Close() error {
	c.closeMutex.Lock()
	defer c.closeMutex.Unlock()

	if c.closed.Swap(true) {
		return nil
	}

	close(c.closeChan)
	return nil
}

func (c *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (c *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (c *mockConn) SetDeadline(_ time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(_ time.Time) error { return nil }

func (c *mockConn) setSendError(err error) {
	c.mu.Lock()
	c.sendErr = err
	c.mu.Unlock()
}

func (c *mockConn) setBlockSend(block bool) {
	c.mu.Lock()
	c.blockSend = block
	c.mu.Unlock()
}

// Test peer creation.
func TestPeerCreation(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() *peer
		validate  func(t *testing.T, p *peer)
	}{
		{
			name: "regular peer with connection",
			setupFunc: func() *peer {
				node := Node{ID: "node1", Mode: "full", Addr: "127.0.0.1:8080"}
				conn := newMockConn("node1", "full")
				return newPeer(node, conn, "inbound")
			},
			validate: func(t *testing.T, p *peer) {
				t.Helper()
				assert.Equal(t, "node1", p.ID)
				assert.Equal(t, "inbound", p.direction)
				assert.True(t, p.IsConnected())
				assert.NotNil(t, p.conn)
				assert.Empty(t, p.addr)
				assert.False(t, p.reconnect)
			},
		},
		{
			name: "outbound peer without initial connection",
			setupFunc: func() *peer {
				node := Node{ID: "node2", Mode: "client"}
				return newOutboundPeer(node, "127.0.0.1:9090")
			},
			validate: func(t *testing.T, p *peer) {
				t.Helper()
				assert.Equal(t, "node2", p.ID)
				assert.Equal(t, "outbound", p.direction)
				assert.False(t, p.IsConnected())
				assert.Nil(t, p.conn)
				assert.Equal(t, "127.0.0.1:9090", p.addr)
				assert.True(t, p.reconnect)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setupFunc()
			defer p.stop()
			tt.validate(t, p)
		})
	}
}

// Test peer connection management.
func TestPeerConnectionManagement(t *testing.T) {
	node := Node{ID: "node1", Mode: "full"}

	t.Run("set connection", func(t *testing.T) {
		p := newOutboundPeer(node, "127.0.0.1:8080")
		defer p.stop()

		assert.False(t, p.IsConnected(), "expected peer to start disconnected")

		conn := newMockConn("node1", "full")
		p.setConnection(conn)

		assert.True(t, p.IsConnected(), "expected peer to be connected after setConnection")
		assert.Equal(t, conn, p.conn, "expected connection to be set")
		assert.Equal(t, 1, p.reconnects)
	})

	t.Run("disconnect", func(t *testing.T) {
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")
		defer p.stop()

		// Set up disconnect callback
		disconnectCalled := false
		p.onDisconnect = func(peer *peer) {
			disconnectCalled = true
			assert.Equal(t, p.ID, peer.ID, "disconnect callback received wrong peer")
		}

		assert.True(t, p.IsConnected(), "expected peer to start connected")

		p.disconnect()

		assert.False(t, p.IsConnected(), "expected peer to be disconnected")
		assert.Nil(t, p.conn, "expected connection to be nil")
		assert.True(t, disconnectCalled, "expected disconnect callback to be called")
		assert.True(t, conn.closed.Load(), "expected connection to be closed")
	})

	t.Run("multiple disconnects", func(t *testing.T) {
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")
		defer p.stop()

		disconnectCount := 0
		p.onDisconnect = func(_ *peer) {
			disconnectCount++
		}

		p.disconnect()
		p.disconnect() // Second disconnect should be safe

		assert.Equal(t, 1, disconnectCount, "expected disconnect callback to be called once")
	})
}

// Test peer info and state tracking.
func TestPeerInfo(t *testing.T) {
	node := Node{ID: "node1", Mode: "full", Addr: "127.0.0.1:8080"}
	conn := newMockConn("node1", "full")
	p := newPeer(node, conn, "outbound")
	defer p.stop()

	// Set some reconnects
	p.mu.Lock()
	p.reconnects = 3
	p.mu.Unlock()

	info := p.Info()

	assert.Equal(t, "node1", info.ID)
	assert.Equal(t, "full", info.Mode)
	assert.Equal(t, "127.0.0.1:8080", info.Addr)
	assert.Equal(t, "outbound", info.Direction)
	assert.Equal(t, 3, info.Reconnects)
	assert.WithinDuration(t, time.Now(), info.ConnectedAt, time.Second)
}

// Test peer sending messages.
func TestPeerSend(t *testing.T) {
	node := Node{ID: "node1", Mode: "full"}

	t.Run("send when connected", func(t *testing.T) {
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")
		defer p.stop()

		msg := "test message"
		err := p.Send(msg)
		assert.NoError(t, err)

		// Verify message was sent
		select {
		case received := <-conn.messages:
			assert.Equal(t, msg, received)
		default:
			t.Error("expected message in connection buffer")
		}
	})

	t.Run("send when disconnected", func(t *testing.T) {
		p := newOutboundPeer(node, "127.0.0.1:8080")
		defer p.stop()

		err := p.Send("test")
		assert.Error(t, err, "expected error when sending to disconnected peer")
	})

	t.Run("send with connection error", func(t *testing.T) {
		conn := newMockConn("node1", "full")
		conn.setSendError(errors.New("send failed"))
		p := newPeer(node, conn, "inbound")
		defer p.stop()

		err := p.Send("test")
		assert.EqualError(t, err, "send failed")
	})
}

// Test peer manager operations.
func TestPeerManager(t *testing.T) {
	t.Run("add peer", func(t *testing.T) {
		m := newPeerManager()

		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")

		connectedCalled := false
		m.onPeerConnected = func(peer *peer) {
			connectedCalled = true
			assert.Equal(t, "node1", peer.ID, "connected callback received wrong peer")
		}

		err := m.AddPeer(p)
		assert.NoError(t, err)
		assert.True(t, connectedCalled, "expected connected callback to be called")

		// Try to add same peer again
		err = m.AddPeer(p)
		assert.Equal(t, ErrPeerExists, err)

		p.stop()
	})

	t.Run("remove peer", func(t *testing.T) {
		m := newPeerManager()

		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")

		_ = m.AddPeer(p)

		err := m.RemovePeer("node1")
		assert.NoError(t, err)

		// Try to remove non-existent peer
		err = m.RemovePeer("node1")
		assert.Equal(t, ErrPeerNotFound, err)

		// Verify peer was stopped
		assert.True(t, conn.closed.Load(), "expected peer connection to be closed")
	})

	t.Run("get peer", func(t *testing.T) {
		m := newPeerManager()

		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")

		_ = m.AddPeer(p)
		defer p.stop()

		// Get existing peer
		retrieved, err := m.GetPeer("node1")
		assert.NoError(t, err)
		assert.Equal(t, p.ID, retrieved.ID, "retrieved wrong peer")

		// Get non-existent peer
		_, err = m.GetPeer("node2")
		assert.Equal(t, ErrPeerNotFound, err)
	})

	t.Run("get all peers", func(t *testing.T) {
		m := newPeerManager()

		// Add multiple peers
		peers := make([]*peer, 3)
		for i := 0; i < 3; i++ {
			node := Node{ID: string(rune('a' + i)), Mode: "full"}
			conn := newMockConn(node.ID, "full")
			peers[i] = newPeer(node, conn, "inbound")
			_ = m.AddPeer(peers[i])
		}

		allPeers := m.GetPeers()
		assert.Len(t, allPeers, 3)

		// Cleanup
		for _, p := range peers {
			p.stop()
		}
	})

	t.Run("get connected peers", func(t *testing.T) {
		m := newPeerManager()

		// Add connected peer
		node1 := Node{ID: "node1", Mode: "full"}
		conn1 := newMockConn("node1", "full")
		p1 := newPeer(node1, conn1, "inbound")
		_ = m.AddPeer(p1)

		// Add disconnected peer
		node2 := Node{ID: "node2", Mode: "full"}
		p2 := newOutboundPeer(node2, "127.0.0.1:8080")
		_ = m.AddPeer(p2)

		connected := m.GetConnectedPeers()
		assert.Len(t, connected, 1)
		assert.Equal(t, "node1", connected[0].ID, "wrong connected peer")

		p1.stop()
		p2.stop()
	})

	t.Run("count peers", func(t *testing.T) {
		m := newPeerManager()

		assert.Equal(t, 0, m.Count())

		// Add peers
		node1 := Node{ID: "node1", Mode: "full"}
		conn1 := newMockConn("node1", "full")
		p1 := newPeer(node1, conn1, "inbound")
		_ = m.AddPeer(p1)

		node2 := Node{ID: "node2", Mode: "full"}
		p2 := newOutboundPeer(node2, "127.0.0.1:8080")
		_ = m.AddPeer(p2)

		assert.Equal(t, 2, m.Count())
		assert.Equal(t, 1, m.ConnectedCount())

		p1.stop()
		p2.stop()
	})
}

// Test broadcast functionality.
func TestBroadcast(t *testing.T) {
	m := newPeerManager()

	// Create multiple connected peers
	conns := make([]*mockConn, 3)
	peers := make([]*peer, 3)
	for i := 0; i < 3; i++ {
		node := Node{ID: string(rune('a' + i)), Mode: "full"}
		conns[i] = newMockConn(node.ID, "full")
		peers[i] = newPeer(node, conns[i], "inbound")
		_ = m.AddPeer(peers[i])
	}

	// Add one disconnected peer
	node4 := Node{ID: "d", Mode: "full"}
	p4 := newOutboundPeer(node4, "127.0.0.1:8080")
	_ = m.AddPeer(p4)

	// Broadcast message
	msg := "broadcast message"
	err := m.Broadcast(msg)
	assert.NoError(t, err)

	// Verify all connected peers received the message
	for i, conn := range conns {
		select {
		case received := <-conn.messages:
			assert.Equal(t, msg, received, "peer %d: expected correct message", i)
		default:
			t.Errorf("peer %d: expected message in buffer", i)
		}
	}

	// Cleanup
	for _, p := range peers {
		p.stop()
	}
	p4.stop()
}

// Test concurrent operations.
func TestConcurrentOperations(t *testing.T) {
	t.Run("concurrent sends", func(t *testing.T) {
		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")
		defer p.stop()

		// Send messages concurrently
		var wg sync.WaitGroup
		numMessages := 100
		wg.Add(numMessages)

		for i := 0; i < numMessages; i++ {
			go func(id int) {
				defer wg.Done()
				msg := id
				err := p.Send(msg)
				assert.NoError(t, err, "send %d failed", id)
			}(i)
		}

		wg.Wait()

		// Verify all messages were sent (order may vary)
		received := make(map[int]bool)
		for i := 0; i < numMessages; i++ {
			select {
			case msg := <-conn.messages:
				if id, ok := msg.(int); ok {
					received[id] = true
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("timeout waiting for messages")
			}
		}

		assert.Len(t, received, numMessages, "expected all unique messages")
	})

	t.Run("concurrent peer manager operations", func(t *testing.T) {
		m := newPeerManager()

		var wg sync.WaitGroup
		numGoroutines := 10
		numOperations := 100

		// Concurrent adds, removes, and gets
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				for j := 0; j < numOperations; j++ {
					nodeID := string(rune('a' + (id*numOperations+j)%26))
					node := Node{ID: nodeID, Mode: "full"}

					switch j % 3 {
					case 0: // Add
						conn := newMockConn(nodeID, "full")
						p := newPeer(node, conn, "inbound")
						_ = m.AddPeer(p)
					case 1: // Get
						_, _ = m.GetPeer(nodeID)
					case 2: // Remove
						_ = m.RemovePeer(nodeID)
					}
				}
			}(i)
		}

		wg.Wait()

		// Stop all remaining peers
		m.Stop()
	})

	t.Run("concurrent disconnect and send", func(t *testing.T) {
		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")
		defer p.stop()

		var wg sync.WaitGroup

		// Start senders
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					_ = p.Send(id*100 + j)
					time.Sleep(time.Microsecond)
				}
			}(i)
		}

		// Disconnect after a short delay
		go func() {
			time.Sleep(50 * time.Microsecond)
			p.disconnect()
		}()

		wg.Wait()
	})
}

// Test lifecycle management.
func TestLifecycleManagement(t *testing.T) {
	t.Run("peer stop", func(t *testing.T) {
		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")

		// Add work to peer
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			<-p.ctx.Done()
		}()

		// Stop should cancel context and wait
		done := make(chan struct{})
		go func() {
			p.stop()
			close(done)
		}()

		select {
		case <-done:
			// Good, stop completed
		case <-time.After(100 * time.Millisecond):
			t.Error("stop timed out")
		}

		// Verify state
		assert.False(t, p.IsConnected(), "expected peer to be disconnected after stop")
		assert.True(t, conn.closed.Load(), "expected connection to be closed after stop")
	})

	t.Run("peer manager stop", func(t *testing.T) {
		m := newPeerManager()

		// Add multiple peers
		peers := make([]*peer, 3)
		conns := make([]*mockConn, 3)
		for i := 0; i < 3; i++ {
			node := Node{ID: string(rune('a' + i)), Mode: "full"}
			conns[i] = newMockConn(node.ID, "full")
			peers[i] = newPeer(node, conns[i], "inbound")
			_ = m.AddPeer(peers[i])
		}

		// Stop manager
		m.Stop()

		// Verify all peers stopped
		for i, conn := range conns {
			assert.True(t, conn.closed.Load(), "peer %d connection not closed", i)
		}

		// Verify manager is empty
		assert.Equal(t, 0, m.Count())
	})

	t.Run("disconnect callback", func(t *testing.T) {
		m := newPeerManager()

		disconnectChan := make(chan string, 1)
		m.onPeerDisconnected = func(p *peer) {
			disconnectChan <- p.ID
		}

		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")
		_ = m.AddPeer(p)

		// Disconnect peer
		p.disconnect()

		select {
		case id := <-disconnectChan:
			assert.Equal(t, "node1", id)
		case <-time.After(100 * time.Millisecond):
			t.Error("disconnect callback not called")
		}

		p.stop()
	})
}

// Test edge cases.
func TestEdgeCases(t *testing.T) {
	t.Run("send to specific peer", func(t *testing.T) {
		m := newPeerManager()

		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")
		_ = m.AddPeer(p)
		defer p.stop()

		// Send to existing peer
		msg := "test message"
		err := m.SendToPeer("node1", msg)
		assert.NoError(t, err)

		// Verify message was sent
		select {
		case received := <-conn.messages:
			assert.Equal(t, msg, received)
		default:
			t.Error("expected message in connection buffer")
		}
	})

	t.Run("send to peer after manager removal", func(t *testing.T) {
		m := newPeerManager()

		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		p := newPeer(node, conn, "inbound")
		_ = m.AddPeer(p)

		// Remove peer from manager
		_ = m.RemovePeer("node1")

		// Try to send through manager
		err := m.SendToPeer("node1", "test")
		assert.Equal(t, ErrPeerNotFound, err)
	})

	t.Run("broadcast with send errors", func(t *testing.T) {
		m := newPeerManager()

		// Create peers with one having send error
		node1 := Node{ID: "node1", Mode: "full"}
		conn1 := newMockConn("node1", "full")
		conn1.setSendError(errors.New("send failed"))
		p1 := newPeer(node1, conn1, "inbound")
		_ = m.AddPeer(p1)

		node2 := Node{ID: "node2", Mode: "full"}
		conn2 := newMockConn("node2", "full")
		p2 := newPeer(node2, conn2, "inbound")
		_ = m.AddPeer(p2)

		// Broadcast should return first error but continue
		err := m.Broadcast("test")
		assert.EqualError(t, err, "send failed")

		// Verify second peer still received message
		select {
		case msg := <-conn2.messages:
			assert.Equal(t, "test", msg)
		default:
			t.Error("expected message in second peer's buffer")
		}

		p1.stop()
		p2.stop()
	})

	t.Run("connection state during concurrent operations", func(t *testing.T) {
		node := Node{ID: "node1", Mode: "full"}
		p := newOutboundPeer(node, "127.0.0.1:8080")
		defer p.stop()

		var wg sync.WaitGroup

		// Concurrent connection state changes
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					conn := newMockConn("node1", "full")
					p.setConnection(conn)
					p.disconnect()
				}
			}()
		}

		// Concurrent state reads
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					p.IsConnected()
					p.Info()
				}
			}()
		}

		wg.Wait()
	})
}

// Test blocking operations.
func TestBlockingOperations(t *testing.T) {
	t.Run("send blocks until connection closed", func(t *testing.T) {
		node := Node{ID: "node1", Mode: "full"}
		conn := newMockConn("node1", "full")
		conn.setBlockSend(true)
		p := newPeer(node, conn, "inbound")
		defer p.stop()

		sendDone := make(chan error, 1)
		go func() {
			err := p.Send("test")
			sendDone <- err
		}()

		// Give send time to block
		time.Sleep(10 * time.Millisecond)

		// Close connection
		_ = conn.Close()

		select {
		case err := <-sendDone:
			assert.Error(t, err, "expected error from blocked send")
		case <-time.After(100 * time.Millisecond):
			t.Error("send did not unblock after connection close")
		}
	})
}
