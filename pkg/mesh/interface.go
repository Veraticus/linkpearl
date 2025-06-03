package mesh

import (
	"context"
	"errors"
	"time"

	"github.com/Veraticus/linkpearl/pkg/transport"
)

// Common errors
var (
	// ErrClosed indicates the topology is closed
	ErrClosed = errors.New("topology closed")

	// ErrPeerExists indicates a peer with the same ID already exists
	ErrPeerExists = errors.New("peer already exists")

	// ErrPeerNotFound indicates the requested peer was not found
	ErrPeerNotFound = errors.New("peer not found")

	// ErrInvalidNode indicates invalid node configuration
	ErrInvalidNode = errors.New("invalid node configuration")
)

// Topology manages the static overlay network of peers
type Topology interface {
	// Start begins accepting connections (for full nodes) and connecting to configured peers
	Start(ctx context.Context) error

	// Stop gracefully shuts down all connections
	Stop() error

	// AddJoinAddr adds an address to maintain a connection to
	AddJoinAddr(addr string) error

	// RemoveJoinAddr removes an address from the join list
	RemoveJoinAddr(addr string) error

	// GetPeer returns information about a connected peer
	GetPeer(nodeID string) (*PeerInfo, error)

	// GetPeers returns information about all connected peers
	GetPeers() map[string]*PeerInfo

	// PeerCount returns the number of connected peers
	PeerCount() int

	// SendToPeer sends a message to a specific peer
	SendToPeer(nodeID string, msg interface{}) error

	// Broadcast sends a message to all connected peers
	Broadcast(msg interface{}) error

	// Events returns a channel of topology events
	Events() <-chan TopologyEvent

	// Messages returns a channel of incoming messages
	Messages() <-chan Message
}

// Node represents a node in the network
type Node struct {
	ID   string `json:"id"`
	Mode string `json:"mode"` // "client" or "full"
	Addr string `json:"addr"` // Listen address (empty for client nodes)
}

// Validate checks if the node configuration is valid
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

// PeerInfo contains information about a connected peer
type PeerInfo struct {
	Node
	ConnectedAt time.Time
	Direction   string // "inbound" or "outbound"
	Reconnects  int    // Number of reconnection attempts
}

// Message represents a message received from a peer
type Message struct {
	From    string      // Node ID of sender
	Type    string      // Message type
	Payload interface{} // Message payload
}

// TopologyConfig holds configuration for creating a topology
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

// DefaultTopologyConfig returns a topology config with sensible defaults
func DefaultTopologyConfig() *TopologyConfig {
	return &TopologyConfig{
		ReconnectInterval:    time.Second,
		MaxReconnectInterval: 5 * time.Minute,
		EventBufferSize:      1000,
		MessageBufferSize:    1000,
		Logger:               DefaultLogger(),
	}
}

// Logger interface for topology logging
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// noopLogger implements Logger with no-op methods
type noopLogger struct{}

func (noopLogger) Debug(msg string, args ...interface{}) {}
func (noopLogger) Info(msg string, args ...interface{})  {}
func (noopLogger) Warn(msg string, args ...interface{})  {}
func (noopLogger) Error(msg string, args ...interface{}) {}

// DefaultLogger returns a no-op logger
func DefaultLogger() Logger {
	return noopLogger{}
}