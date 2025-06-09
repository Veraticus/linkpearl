// Package sync provides the clipboard synchronization engine for linkpearl.
// It implements a distributed synchronization protocol that ensures clipboard
// content consistency across all nodes in the mesh network.
//
// The sync engine handles:
//   - Real-time clipboard monitoring and change detection
//   - Distributed state synchronization using a gossip-like protocol
//   - Conflict resolution using last-write-wins with timestamp ordering
//   - Message deduplication to prevent sync loops and redundant updates
//   - Network partition tolerance with eventual consistency
//
// Architecture Overview:
//
// The sync engine operates as an event-driven system that responds to:
//   - Local clipboard changes from the OS
//   - Remote clipboard updates from peer nodes
//   - Topology changes (peer connections/disconnections)
//
// Conflict Resolution:
//
// When multiple nodes modify their clipboards simultaneously, conflicts are
// resolved using a last-write-wins strategy based on timestamps. If timestamps
// are identical (extremely rare), node IDs are used as a deterministic tiebreaker.
//
// Deduplication:
//
// To prevent sync loops and redundant processing, the engine maintains an LRU
// cache of recently seen clipboard checksums. This cache helps identify:
//   - Echo messages (our own changes coming back from the network)
//   - Duplicate messages from multiple paths in the mesh
//   - Rapid successive changes that might cause loops
//
// Example Usage:
//
//	config := &sync.Config{
//	    NodeID:    "node-1",
//	    Clipboard: clipboard.NewDarwinClipboard(),
//	    Topology:  mesh.NewTopology(...),
//	    Logger:    logger,
//	}
//
//	engine, err := sync.NewEngine(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Run the sync engine
//	if err := engine.Run(ctx); err != nil {
//	    log.Fatal(err)
//	}
package sync

import (
	"context"
	"errors"
	"time"

	"github.com/Veraticus/linkpearl/pkg/clipboard"
	"github.com/Veraticus/linkpearl/pkg/mesh"
)

var (
	// ErrInvalidMessage indicates a malformed message.
	ErrInvalidMessage = errors.New("invalid message")

	// ErrChecksumMismatch indicates content doesn't match checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrDuplicateMessage indicates we've already seen this message.
	ErrDuplicateMessage = errors.New("duplicate message")

	// ErrOldMessage indicates message is older than our current state.
	ErrOldMessage = errors.New("message older than current state")

	// ErrSyncLoop indicates potential sync loop detected.
	ErrSyncLoop = errors.New("sync loop detected")
)

// Engine coordinates clipboard synchronization across the mesh.
type Engine interface {
	// Run starts the sync engine main loop
	Run(ctx context.Context) error

	// Stats returns current engine statistics
	Stats() *Stats

	// Topology returns the underlying mesh topology
	Topology() Topology

	// SetClipboard enqueues a local clipboard change for synchronization.
	// This method is used by both the clipboard watcher and the API server
	// to notify the sync engine of new clipboard content.
	SetClipboard(content string) error
}

// Stats contains sync engine statistics.
type Stats struct {
	LastLocalChange   time.Time
	StartTime         time.Time
	LastSyncTime      time.Time
	LastRemoteChange  time.Time
	LocalChanges      uint64
	ConflictsWon      uint64
	ConflictsLost     uint64
	SendErrors        uint64
	ReceiveErrors     uint64
	RemoteChanges     uint64
	MessagesSent      uint64
	MessagesDuplicate uint64
	MessagesReceived  uint64
}

// Config holds sync engine configuration.
type Config struct {
	Clipboard         clipboard.Clipboard
	Topology          mesh.Topology
	Logger            Logger
	NodeID            string
	DedupeSize        int
	SyncLoopWindow    time.Duration
	MinChangeInterval time.Duration
	CommandTimeout    time.Duration
	MaxClipboardSize  int
}

// Logger interface for sync engine logging.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// Validate checks if config is valid.
func (c *Config) Validate() error {
	if c.NodeID == "" {
		return errors.New("node ID is required")
	}
	if c.Clipboard == nil {
		return errors.New("clipboard is required")
	}
	if c.Topology == nil {
		return errors.New("topology is required")
	}

	// Apply defaults
	if c.DedupeSize <= 0 {
		c.DedupeSize = 1000
	}
	if c.SyncLoopWindow <= 0 {
		c.SyncLoopWindow = 500 * time.Millisecond
	}
	if c.MinChangeInterval <= 0 {
		c.MinChangeInterval = 100 * time.Millisecond
	}
	if c.CommandTimeout <= 0 {
		c.CommandTimeout = 5 * time.Second
	}
	if c.MaxClipboardSize <= 0 {
		c.MaxClipboardSize = clipboard.MaxClipboardSize
	}
	if c.Logger == nil {
		c.Logger = &noopLogger{}
	}

	return nil
}

// noopLogger implements Logger with no operations.
type noopLogger struct{}

func (n *noopLogger) Debug(_ string, _ ...any) {}
func (n *noopLogger) Info(_ string, _ ...any)  {}
func (n *noopLogger) Error(_ string, _ ...any) {}

// Topology provides information about the mesh network topology.
// This is a simplified interface for querying topology state.
type Topology interface {
	// ListenAddr returns the address the node is listening on, or empty string if not listening
	ListenAddr() string

	// ConnectedPeers returns a list of connected peer node IDs
	ConnectedPeers() []string

	// JoinAddresses returns the list of addresses this node is configured to join
	JoinAddresses() []string
}

// MockTopology implements Topology for testing.
type MockTopology struct {
	ListenAddress string
	Peers         []string
	JoinAddrs     []string
}

// ListenAddr returns the mock listen address.
func (m *MockTopology) ListenAddr() string {
	return m.ListenAddress
}

// ConnectedPeers returns the mock connected peers.
func (m *MockTopology) ConnectedPeers() []string {
	return m.Peers
}

// JoinAddresses returns the mock join addresses.
func (m *MockTopology) JoinAddresses() []string {
	return m.JoinAddrs
}
