package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Veraticus/linkpearl/pkg/transport"
)

// topology implements the Topology interface
type topology struct {
	self      Node
	transport transport.Transport
	config    *TopologyConfig

	// State
	mu       sync.RWMutex
	started  bool
	closed   bool
	joinAddrs []string

	// Peer management
	peers    *peerManager
	backoffs *backoffManager

	// Event and message handling
	events   *eventPump
	messages chan Message
	router   *messageRouter

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewTopology creates a new topology
func NewTopology(config *TopologyConfig) (Topology, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Validate node configuration
	if err := config.Self.Validate(); err != nil {
		return nil, fmt.Errorf("invalid node configuration: %w", err)
	}

	// Apply defaults
	if config.ReconnectInterval <= 0 {
		config.ReconnectInterval = time.Second
	}
	if config.MaxReconnectInterval <= 0 {
		config.MaxReconnectInterval = 5 * time.Minute
	}
	if config.EventBufferSize <= 0 {
		config.EventBufferSize = 1000
	}
	if config.MessageBufferSize <= 0 {
		config.MessageBufferSize = 1000
	}
	if config.Logger == nil {
		config.Logger = DefaultLogger()
	}

	ctx, cancel := context.WithCancel(context.Background())
	t := &topology{
		self:      config.Self,
		transport: config.Transport,
		config:    config,
		joinAddrs: make([]string, 0, len(config.JoinAddrs)),
		peers:     newPeerManager(),
		backoffs:  newBackoffManager(DefaultBackoff),
		events:    newEventPump(config.EventBufferSize),
		messages:  make(chan Message, config.MessageBufferSize),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Copy join addresses
	t.joinAddrs = append(t.joinAddrs, config.JoinAddrs...)

	// Set up message router
	t.router = newMessageRouter(t.self.ID, func(nodeID string, data []byte) error {
		if nodeID == "" {
			// Broadcast
			return t.peers.Broadcast(data)
		}
		return t.peers.SendToPeer(nodeID, data)
	})

	// Register message handlers
	t.registerMessageHandlers()

	// Set up peer callbacks
	t.peers.onPeerConnected = t.handlePeerConnected
	t.peers.onPeerDisconnected = t.handlePeerDisconnected

	return t, nil
}

// Start begins accepting connections and connecting to configured peers
func (t *topology) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.started {
		return fmt.Errorf("topology already started")
	}
	if t.closed {
		return ErrClosed
	}

	t.started = true

	// Start listening if we're a full node
	if t.self.Mode == "full" && t.self.Addr != "" {
		if err := t.transport.Listen(t.self.Addr); err != nil {
			return fmt.Errorf("failed to listen on %s: %w", t.self.Addr, err)
		}

		// Start accepting connections
		t.wg.Add(1)
		go t.acceptLoop()
	}

	// Start connecting to configured peers
	for _, addr := range t.joinAddrs {
		t.wg.Add(1)
		go t.maintainConnection(addr)
	}

	t.config.Logger.Info("topology started", "node", t.self.ID, "mode", t.self.Mode)
	return nil
}

// Stop gracefully shuts down all connections
func (t *topology) Stop() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	t.mu.Unlock()

	// Cancel context to stop all goroutines
	t.cancel()

	// Close transport
	t.transport.Close()

	// Stop all peers
	t.peers.Stop()

	// Close channels
	close(t.messages)
	t.events.Close()

	// Wait for goroutines
	t.wg.Wait()

	t.config.Logger.Info("topology stopped", "node", t.self.ID)
	return nil
}

// AddJoinAddr adds an address to maintain a connection to
func (t *topology) AddJoinAddr(addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	// Check if already exists
	for _, existing := range t.joinAddrs {
		if existing == addr {
			return nil // Already exists
		}
	}

	t.joinAddrs = append(t.joinAddrs, addr)

	// If already started, begin maintaining connection
	if t.started {
		t.wg.Add(1)
		go t.maintainConnection(addr)
	}

	return nil
}

// RemoveJoinAddr removes an address from the join list
func (t *topology) RemoveJoinAddr(addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	// Find and remove
	for i, existing := range t.joinAddrs {
		if existing == addr {
			t.joinAddrs = append(t.joinAddrs[:i], t.joinAddrs[i+1:]...)
			// Note: Existing connections will continue until they disconnect
			return nil
		}
	}

	return nil
}

// GetPeer returns information about a connected peer
func (t *topology) GetPeer(nodeID string) (*PeerInfo, error) {
	p, err := t.peers.GetPeer(nodeID)
	if err != nil {
		return nil, err
	}
	return p.Info(), nil
}

// GetPeers returns information about all connected peers
func (t *topology) GetPeers() map[string]*PeerInfo {
	peers := t.peers.GetPeers()
	result := make(map[string]*PeerInfo, len(peers))
	for _, p := range peers {
		result[p.ID] = p.Info()
	}
	return result
}

// PeerCount returns the number of connected peers
func (t *topology) PeerCount() int {
	return t.peers.ConnectedCount()
}

// SendToPeer sends a message to a specific peer
func (t *topology) SendToPeer(nodeID string, msg interface{}) error {
	return t.peers.SendToPeer(nodeID, msg)
}

// Broadcast sends a message to all connected peers
func (t *topology) Broadcast(msg interface{}) error {
	return t.peers.Broadcast(msg)
}

// Events returns a channel of topology events
func (t *topology) Events() <-chan TopologyEvent {
	ch := make(chan TopologyEvent, 100)
	t.events.Subscribe(ch)
	return ch
}

// Messages returns a channel of incoming messages
func (t *topology) Messages() <-chan Message {
	return t.messages
}

// acceptLoop accepts incoming connections
func (t *topology) acceptLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		conn, err := t.transport.Accept()
		if err != nil {
			select {
			case <-t.ctx.Done():
				return
			default:
				t.config.Logger.Error("failed to accept connection", "error", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		// Handle the connection
		t.wg.Add(1)
		go t.handleIncomingConnection(conn)
	}
}

// handleIncomingConnection handles a new incoming connection
func (t *topology) handleIncomingConnection(conn transport.Conn) {
	defer t.wg.Done()

	nodeID := conn.NodeID()
	t.config.Logger.Info("accepted connection", "from", nodeID)

	// Create peer
	p := newPeer(
		Node{
			ID:   nodeID,
			Mode: conn.Mode(),
			Addr: "", // We don't know their listen address
		},
		conn,
		"inbound",
	)

	// Add to peer manager
	if err := t.peers.AddPeer(p); err != nil {
		t.config.Logger.Error("failed to add peer", "node", nodeID, "error", err)
		conn.Close()
		return
	}

	// Handle the connection
	t.handleConnection(p)
}

// maintainConnection maintains a connection to a configured address
func (t *topology) maintainConnection(addr string) {
	defer t.wg.Done()

	backoff := t.backoffs.Get(addr)

	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		// Attempt connection
		t.config.Logger.Debug("connecting to", "addr", addr)
		
		ctx, cancel := context.WithTimeout(t.ctx, 30*time.Second)
		conn, err := t.transport.Connect(ctx, addr)
		cancel()

		if err != nil {
			t.config.Logger.Error("failed to connect", "addr", addr, "error", err)
			
			// Wait with backoff
			duration := backoff.Next()
			t.config.Logger.Debug("reconnecting in", "addr", addr, "duration", duration)
			
			select {
			case <-t.ctx.Done():
				return
			case <-time.After(duration):
				continue
			}
		}

		// Success - reset backoff
		backoff.Reset()
		nodeID := conn.NodeID()
		t.config.Logger.Info("connected to", "addr", addr, "node", nodeID)

		// Check if we already have this peer
		if existing, _ := t.peers.GetPeer(nodeID); existing != nil {
			t.config.Logger.Warn("already connected to peer", "node", nodeID)
			conn.Close()
			// Wait a bit before retrying
			select {
			case <-t.ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Create peer
		p := newOutboundPeer(
			Node{
				ID:   nodeID,
				Mode: conn.Mode(),
				Addr: conn.Version(), // Hack: we could get listen addr from version field
			},
			addr,
		)
		p.setConnection(conn)

		// Add to peer manager
		if err := t.peers.AddPeer(p); err != nil {
			t.config.Logger.Error("failed to add peer", "node", nodeID, "error", err)
			conn.Close()
			continue
		}

		// Handle the connection
		t.handleConnection(p)

		// Connection closed, loop will retry
		t.config.Logger.Info("connection closed", "addr", addr, "node", nodeID)
	}
}

// handleConnection handles a peer connection
func (t *topology) handleConnection(p *peer) {
	// Send our peer list
	t.sendPeerList(p)

	// Read messages until connection closes
	for {
		var msg interface{}
		if err := p.conn.Receive(&msg); err != nil {
			t.config.Logger.Debug("connection closed", "node", p.ID, "error", err)
			break
		}

		// Process the message
		if data, ok := msg.([]byte); ok {
			if err := t.router.ProcessMessage(data); err != nil {
				t.config.Logger.Error("failed to process message", "node", p.ID, "error", err)
			}
		} else if data, err := json.Marshal(msg); err == nil {
			if err := t.router.ProcessMessage(data); err != nil {
				t.config.Logger.Error("failed to process message", "node", p.ID, "error", err)
			}
		}
	}

	// Disconnect the peer
	p.disconnect()
}

// handlePeerConnected handles a peer connection event
func (t *topology) handlePeerConnected(p *peer) {
	t.events.Publish(TopologyEvent{
		Type: PeerConnected,
		Peer: p.Node,
		Time: time.Now(),
	})
}

// handlePeerDisconnected handles a peer disconnection event
func (t *topology) handlePeerDisconnected(p *peer) {
	t.events.Publish(TopologyEvent{
		Type: PeerDisconnected,
		Peer: p.Node,
		Time: time.Now(),
	})

	// Remove from peer manager
	t.peers.RemovePeer(p.ID)
}

// sendPeerList sends the current peer list to a peer
func (t *topology) sendPeerList(p *peer) {
	peers := t.peers.GetConnectedPeers()
	nodes := make([]Node, 0, len(peers)+1)

	// Add ourselves
	nodes = append(nodes, t.self)

	// Add connected peers
	for _, peer := range peers {
		if peer.ID != p.ID { // Don't include the recipient
			nodes = append(nodes, peer.Node)
		}
	}

	msg := PeerListMessage{Peers: nodes}
	if err := t.router.SendToPeer(p.ID, MessageTypePeerList, msg); err != nil {
		t.config.Logger.Error("failed to send peer list", "to", p.ID, "error", err)
	}
}

// registerMessageHandlers registers handlers for mesh messages
func (t *topology) registerMessageHandlers() {
	// Handle peer list messages
	t.router.handler.Register(MessageTypePeerList, func(from string, payload json.RawMessage) error {
		var msg PeerListMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return err
		}

		t.config.Logger.Debug("received peer list", "from", from, "peers", len(msg.Peers))
		// Note: We don't act on this information (static topology)
		return nil
	})

	// Handle ping messages
	t.router.handler.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		var msg PingMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return err
		}

		// Send pong response
		pong := PongMessage{Timestamp: msg.Timestamp}
		return t.router.SendToPeer(from, MessageTypePong, pong)
	})

	// Handle pong messages
	t.router.handler.Register(MessageTypePong, func(from string, payload json.RawMessage) error {
		// Just log it for now
		t.config.Logger.Debug("received pong", "from", from)
		return nil
	})

	// Forward other messages to the message channel
	defaultHandler := func(msgType MessageType) func(string, json.RawMessage) error {
		return func(from string, payload json.RawMessage) error {
			select {
			case t.messages <- Message{
				From:    from,
				Type:    string(msgType),
				Payload: payload,
			}:
			case <-t.ctx.Done():
				return t.ctx.Err()
			default:
				// Message buffer full, drop message
				t.config.Logger.Warn("message buffer full, dropping message", "from", from, "type", msgType)
			}
			return nil
		}
	}

	// Register default handler for clipboard messages
	t.router.handler.Register(MessageTypeClipboard, defaultHandler(MessageTypeClipboard))
}