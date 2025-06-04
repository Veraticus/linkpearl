// peer.go implements peer management for the mesh topology.
// This file contains the peer representation and the peer manager that tracks
// all connected peers in the network.

package mesh

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Veraticus/linkpearl/pkg/transport"
)

// peer represents a connected peer in the topology.
// It encapsulates the connection state, transport connection, and metadata
// about a remote node. Peers can be either inbound (accepted connections)
// or outbound (initiated connections).
//
// The peer struct is thread-safe and manages its own lifecycle. For outbound
// peers, it tracks reconnection information to support automatic reconnection
// on disconnect.
type peer struct {
	Node
	conn      transport.Conn
	direction string // "inbound" or "outbound"

	// For outbound connections that we should reconnect
	addr      string
	reconnect bool

	// State
	mu          sync.RWMutex
	connected   bool
	connectedAt time.Time
	reconnects  int

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Callbacks
	onDisconnect func(*peer)
}

// newPeer creates a new peer
func newPeer(node Node, conn transport.Conn, direction string) *peer {
	ctx, cancel := context.WithCancel(context.Background())
	return &peer{
		Node:        node,
		conn:        conn,
		direction:   direction,
		connected:   true,
		connectedAt: time.Now(),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// newOutboundPeer creates a new outbound peer that will reconnect
func newOutboundPeer(node Node, addr string) *peer {
	ctx, cancel := context.WithCancel(context.Background())
	return &peer{
		Node:      node,
		direction: "outbound",
		addr:      addr,
		reconnect: true,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Info returns information about the peer
func (p *peer) Info() *PeerInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return &PeerInfo{
		Node:        p.Node,
		ConnectedAt: p.connectedAt,
		Direction:   p.direction,
		Reconnects:  p.reconnects,
	}
}

// IsConnected returns true if the peer is connected
func (p *peer) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connected
}

// Send sends a message to the peer
func (p *peer) Send(msg interface{}) error {
	p.mu.RLock()
	if !p.connected || p.conn == nil {
		p.mu.RUnlock()
		return fmt.Errorf("peer %s not connected", p.ID)
	}
	conn := p.conn
	p.mu.RUnlock()

	return conn.Send(msg)
}

// Receive receives a message from the peer
func (p *peer) Receive(msg interface{}) error {
	p.mu.RLock()
	if !p.connected || p.conn == nil {
		p.mu.RUnlock()
		return fmt.Errorf("peer %s not connected", p.ID)
	}
	conn := p.conn
	p.mu.RUnlock()

	return conn.Receive(msg)
}

// setConnection updates the peer's connection
func (p *peer) setConnection(conn transport.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.conn = conn
	p.connected = true
	p.connectedAt = time.Now()
	p.reconnects++
}

// disconnect marks the peer as disconnected
func (p *peer) disconnect() {
	p.mu.Lock()
	wasConnected := p.connected
	p.connected = false
	oldConn := p.conn
	p.conn = nil
	onDisconnect := p.onDisconnect
	p.mu.Unlock()

	// Close the connection if we have one
	if oldConn != nil {
		_ = oldConn.Close()
	}

	// Notify disconnect callback if we were connected
	if wasConnected && onDisconnect != nil {
		onDisconnect(p)
	}
}

// stop stops the peer
func (p *peer) stop() {
	p.cancel()
	p.disconnect()
	p.wg.Wait()
}

// peerManager manages all peers in the topology.
// It provides thread-safe operations for adding, removing, and querying peers,
// as well as sending messages to individual peers or broadcasting to all peers.
//
// The peer manager ensures that only one peer connection exists per node ID,
// preventing duplicate connections. It also provides callbacks for peer lifecycle
// events, allowing the topology to react to connections and disconnections.
type peerManager struct {
	peers map[string]*peer
	mu    sync.RWMutex

	// Callbacks
	onPeerConnected    func(*peer)
	onPeerDisconnected func(*peer)
}

// newPeerManager creates a new peer manager
func newPeerManager() *peerManager {
	return &peerManager{
		peers: make(map[string]*peer),
	}
}

// AddPeer adds a peer to the manager
func (m *peerManager) AddPeer(p *peer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.peers[p.ID]; exists {
		return ErrPeerExists
	}

	// Set up callbacks
	p.onDisconnect = func(peer *peer) {
		if m.onPeerDisconnected != nil {
			m.onPeerDisconnected(peer)
		}
	}

	m.peers[p.ID] = p

	// Notify connected callback
	if p.IsConnected() && m.onPeerConnected != nil {
		m.onPeerConnected(p)
	}

	return nil
}

// RemovePeer removes a peer from the manager
func (m *peerManager) RemovePeer(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.peers[nodeID]
	if !exists {
		return ErrPeerNotFound
	}

	delete(m.peers, nodeID)
	p.stop()

	return nil
}

// GetPeer returns a peer by ID
func (m *peerManager) GetPeer(nodeID string) (*peer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.peers[nodeID]
	if !exists {
		return nil, ErrPeerNotFound
	}

	return p, nil
}

// GetPeers returns all peers
func (m *peerManager) GetPeers() []*peer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peers := make([]*peer, 0, len(m.peers))
	for _, p := range m.peers {
		peers = append(peers, p)
	}

	return peers
}

// GetConnectedPeers returns all connected peers
func (m *peerManager) GetConnectedPeers() []*peer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peers := make([]*peer, 0, len(m.peers))
	for _, p := range m.peers {
		if p.IsConnected() {
			peers = append(peers, p)
		}
	}

	return peers
}

// Count returns the total number of peers
func (m *peerManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}

// ConnectedCount returns the number of connected peers
func (m *peerManager) ConnectedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, p := range m.peers {
		if p.IsConnected() {
			count++
		}
	}
	return count
}

// SendToPeer sends a message to a specific peer
func (m *peerManager) SendToPeer(nodeID string, msg interface{}) error {
	p, err := m.GetPeer(nodeID)
	if err != nil {
		return err
	}

	return p.Send(msg)
}

// Broadcast sends a message to all connected peers
func (m *peerManager) Broadcast(msg interface{}) error {
	peers := m.GetConnectedPeers()

	var firstErr error
	for _, p := range peers {
		if err := p.Send(msg); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// Stop stops all peers
func (m *peerManager) Stop() {
	m.mu.Lock()
	peers := make([]*peer, 0, len(m.peers))
	for _, p := range m.peers {
		peers = append(peers, p)
	}
	m.peers = make(map[string]*peer)
	m.mu.Unlock()

	// Stop all peers
	for _, p := range peers {
		p.stop()
	}
}