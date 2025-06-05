// Package mesh provides a P2P mesh topology implementation for creating static overlay networks.
// The mesh package implements a simple but robust peer-to-peer networking layer that supports
// both full nodes (which can accept incoming connections) and client nodes (which only make
// outgoing connections).
//
// # Architecture Overview
//
// The mesh topology is designed as a static overlay network where nodes maintain persistent
// connections to a configured set of peers. Unlike dynamic P2P networks, the topology doesn't
// perform peer discovery or gossip protocols - instead, it focuses on maintaining reliable
// connections to explicitly configured peers.
//
// # Node Types
//
// The mesh supports two types of nodes:
//
//   - Full nodes: Can both accept incoming connections and make outgoing connections.
//     These nodes have a listen address and can act as connection points for client nodes.
//
//   - Client nodes: Can only make outgoing connections. These are typically end-user
//     applications that connect to full nodes but don't accept incoming connections.
//
// # Core Components
//
// The package consists of several key components:
//
//   - Topology: The main interface that manages the overlay network, peer connections,
//     and message routing. It handles connection lifecycle, reconnection logic, and
//     provides APIs for sending messages.
//
//   - Peer Manager: Manages the set of connected peers, tracks their state, and handles
//     connection/disconnection events. It ensures peers are unique by node ID.
//
//   - Message Router: Routes messages between peers and local handlers. It supports both
//     unicast (peer-to-peer) and broadcast messaging patterns.
//
//   - Event System: Provides an event-driven interface for monitoring topology changes
//     such as peer connections and disconnections. Events are buffered and delivered
//     asynchronously to subscribers.
//
//   - Backoff Manager: Implements exponential backoff with jitter for connection retry
//     logic. This ensures the system doesn't overwhelm peers with rapid reconnection
//     attempts during failures.
//
// # Connection Management
//
// The mesh topology maintains persistent connections to configured peers:
//
//   - Outbound connections are automatically retried with exponential backoff if they fail
//   - Inbound connections are accepted from any peer (but deduplicated by node ID)
//   - Each peer connection is bidirectional once established
//   - The topology sends periodic keepalive messages to detect dead connections
//
// # Message Protocol
//
// Messages in the mesh use a simple JSON-based protocol:
//
//   - Each message has a type, sender ID, and payload
//   - The mesh provides built-in message types for peer discovery and health checks
//   - Applications can register custom message handlers for their own message types
//   - Messages can be sent to specific peers or broadcast to all connected peers
//
// # Thread Safety
//
// All public APIs in the mesh package are thread-safe. The implementation uses fine-grained
// locking to ensure concurrent access is safe while maintaining good performance.
//
// # Example Usage
//
//	// Create a full node that accepts connections
//	config := mesh.DefaultTopologyConfig()
//	config.Self = mesh.Node{
//	    ID:   "node1",
//	    Mode: "full",
//	    Addr: "localhost:8080",
//	}
//	config.Transport = tcpTransport
//
//	topology, err := mesh.NewTopology(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Start the topology
//	if err := topology.Start(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Subscribe to events
//	events := topology.Events()
//	go func() {
//	    for event := range events {
//	        log.Printf("Event: %v", event)
//	    }
//	}()
//
//	// Send a message to a peer
//	err = topology.SendToPeer("node2", MyMessage{Data: "hello"})
//
// # Design Principles
//
// The mesh package follows several key design principles:
//
//  1. Simplicity: The topology is intentionally simple, avoiding complex protocols
//     in favor of reliability and ease of understanding.
//
//  2. Static Configuration: Peers are explicitly configured rather than discovered,
//     making the network topology predictable and debuggable.
//
//  3. Resilience: Automatic reconnection with backoff ensures the mesh heals itself
//     after network failures or peer restarts.
//
//  4. Extensibility: The message handler system allows applications to implement
//     their own protocols on top of the mesh.
//
//  5. Observable: The event system provides visibility into topology changes,
//     making it easy to monitor and debug the network.
package mesh

import (
	"context"
	"errors"
	"time"

	"github.com/Veraticus/linkpearl/pkg/transport"
)

// Common errors.
var (
	// ErrClosed indicates the topology is closed.
	ErrClosed = errors.New("topology closed")

	// ErrPeerExists indicates a peer with the same ID already exists.
	ErrPeerExists = errors.New("peer already exists")

	// ErrPeerNotFound indicates the requested peer was not found.
	ErrPeerNotFound = errors.New("peer not found")

	// ErrInvalidNode indicates invalid node configuration.
	ErrInvalidNode = errors.New("invalid node configuration")
)

// Topology manages the static overlay network of peers.
// It provides the main interface for interacting with the mesh network,
// handling peer connections, message routing, and topology monitoring.
type Topology interface {
	// Start begins accepting connections (for full nodes) and connecting to configured peers.
	// This method must be called before the topology becomes operational. For full nodes,
	// it starts listening on the configured address. For all nodes, it initiates connections
	// to the configured join addresses.
	//
	// The provided context is not stored; it's only used for the startup process.
	// Returns an error if the topology is already started or if listening fails.
	Start(ctx context.Context) error

	// Stop gracefully shuts down all connections and releases resources.
	// This method blocks until all connections are closed and goroutines have terminated.
	// After Stop returns, the topology cannot be restarted and should be discarded.
	Stop() error

	// AddJoinAddr adds an address to maintain a connection to.
	// The topology will attempt to connect to this address and maintain the connection
	// with automatic reconnection on failure. If the topology is already started,
	// connection attempts begin immediately.
	//
	// Adding an address that already exists is a no-op and returns nil.
	AddJoinAddr(addr string) error

	// RemoveJoinAddr removes an address from the join list.
	// This stops future reconnection attempts to the address, but does not
	// immediately disconnect if a connection is currently established.
	// The connection will be closed when it naturally disconnects.
	//
	// Removing an address that doesn't exist is a no-op and returns nil.
	RemoveJoinAddr(addr string) error

	// GetPeer returns information about a connected peer.
	// Returns ErrPeerNotFound if the peer is not currently connected.
	// The returned PeerInfo is a snapshot and won't reflect future changes.
	GetPeer(nodeID string) (*PeerInfo, error)

	// GetPeers returns information about all connected peers.
	// The returned map is a snapshot of the current state. The map keys are node IDs.
	// Returns an empty map if no peers are connected.
	GetPeers() map[string]*PeerInfo

	// PeerCount returns the number of connected peers.
	// This only counts peers that are currently connected, not configured addresses
	// that may be in a reconnecting state.
	PeerCount() int

	// SendToPeer sends a message to a specific peer.
	// The message is serialized to JSON before sending. Returns an error if the
	// peer is not connected or if serialization fails.
	//
	// This method is non-blocking; the message is queued for delivery.
	SendToPeer(nodeID string, msg interface{}) error

	// Broadcast sends a message to all connected peers.
	// The message is serialized to JSON before sending. Returns an error if
	// serialization fails. If sending fails to some peers, the first error is returned,
	// but delivery is still attempted to all peers.
	//
	// This method is non-blocking; messages are queued for delivery.
	Broadcast(msg interface{}) error

	// Events returns a channel of topology events.
	// The channel is buffered and receives events for peer connections and disconnections.
	// The caller should consume events promptly to avoid blocking event delivery.
	// The channel is closed when the topology stops.
	//
	// Multiple calls return independent channels that each receive all events.
	Events() <-chan TopologyEvent

	// Messages returns a channel of incoming messages.
	// The channel receives messages that aren't handled by built-in handlers.
	// The caller is responsible for type-switching on the message payload.
	// The channel is closed when the topology stops.
	//
	// Only messages with types not handled internally are delivered to this channel.
	Messages() <-chan Message
}

// Node represents a node in the network.
type Node struct {
	ID   string `json:"id"`
	Mode string `json:"mode"` // "client" or "full"
	Addr string `json:"addr"` // Listen address (empty for client nodes)
}

// Validate checks if the node configuration is valid.
func (n *Node) Validate() error {
	if n.ID == "" {
		return errors.New("node ID is required")
	}
	if n.Mode != "client" && n.Mode != "full" {
		return errors.New("node mode must be 'client' or 'full'")
	}
	if n.Mode == "full" && n.Addr == "" {
		return errors.New("full nodes must have a listen address")
	}
	if n.Mode == "client" && n.Addr != "" {
		return errors.New("client nodes should not have a listen address")
	}
	return nil
}

// PeerInfo contains information about a connected peer.
type PeerInfo struct {
	Node
	ConnectedAt time.Time
	Direction   string // "inbound" or "outbound"
	Reconnects  int    // Number of reconnection attempts
}

// Message represents a message received from a peer.
type Message struct {
	From    string      // Node ID of sender
	Type    string      // Message type
	Payload interface{} // Message payload
}

// TopologyConfig holds configuration for creating a topology.
type TopologyConfig struct {
	// Self is this node's information
	Self Node

	// Transport is the underlying transport to use
	Transport transport.Transport

	// JoinAddrs are the initial addresses to connect to
	JoinAddrs []string

	// ReconnectInterval is the initial reconnection interval
	ReconnectInterval time.Duration

	// MaxReconnectInterval is the maximum reconnection interval
	MaxReconnectInterval time.Duration

	// EventBufferSize is the size of the event buffer
	EventBufferSize int

	// MessageBufferSize is the size of the message buffer
	MessageBufferSize int

	// Logger provides optional logging
	Logger Logger
}

// DefaultTopologyConfig returns a topology config with sensible defaults.
func DefaultTopologyConfig() *TopologyConfig {
	return &TopologyConfig{
		ReconnectInterval:    time.Second,
		MaxReconnectInterval: 5 * time.Minute,
		EventBufferSize:      1000,
		MessageBufferSize:    1000,
		Logger:               DefaultLogger(),
	}
}

// Logger interface for topology logging.
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// noopLogger implements Logger with no-op methods.
type noopLogger struct{}

func (noopLogger) Debug(_ string, _ ...interface{}) {}
func (noopLogger) Info(_ string, _ ...interface{})  {}
func (noopLogger) Warn(_ string, _ ...interface{})  {}
func (noopLogger) Error(_ string, _ ...interface{}) {}

// DefaultLogger returns a no-op logger.
func DefaultLogger() Logger {
	return noopLogger{}
}
