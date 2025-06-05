package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Veraticus/linkpearl/pkg/clipboard"
	"github.com/Veraticus/linkpearl/pkg/mesh"
)

// engine implements the Engine interface and serves as the core synchronization
// component of linkpearl. It coordinates clipboard state across all nodes in the
// mesh network using an event-driven architecture.
//
// The engine maintains several key responsibilities:
//
// State Management:
//   - Tracks current clipboard content, checksum, and timestamp
//   - Maintains a dedupe cache to prevent processing duplicate messages
//   - Records statistics for monitoring and debugging
//
// Event Processing:
//   - Local clipboard changes: Detected via OS-specific clipboard monitoring
//   - Remote messages: Clipboard updates from peer nodes
//   - Topology events: Peer connections and disconnections
//
// Synchronization Protocol:
//   - Broadcasts local changes to all connected peers
//   - Applies remote changes based on conflict resolution rules
//   - Prevents sync loops through deduplication and timing checks
//
// Thread Safety:
//   - Uses mutex protection for shared state
//   - Atomic operations for statistics counters
//   - Concurrent-safe message processing
//
// Internal Architecture:
//
// The engine uses three main channels for event processing:
//  1. clipCh: Receives local clipboard changes from the OS
//  2. msgCh: Receives sync messages from peer nodes
//  3. eventCh: Receives topology change notifications
//
// These channels are processed in a select loop that runs until the context
// is cancelled, ensuring graceful shutdown and resource cleanup.
type engine struct {
	config    *Config
	clipboard clipboard.Clipboard
	topology  mesh.Topology
	logger    Logger

	// Deduplication
	dedupe *lruCache

	// Current state
	mu        sync.RWMutex
	current   string
	checksum  string
	timestamp int64

	// Statistics
	stats Stats

	// Sync loop detection
	lastLocalChange  time.Time
	lastRemoteChange time.Time
}

// NewEngine creates a new sync engine with the provided configuration.
// It validates the config, initializes the deduplication cache, and sets up
// the internal state required for synchronization.
//
// The engine starts in an uninitialized state and must be started with Run()
// to begin synchronization. This separation allows for proper setup and
// dependency injection before starting the main event loop.
//
// Returns an error if the configuration is invalid or if initialization fails.
func NewEngine(config *Config) (Engine, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create dedupe cache
	dedupe, err := newLRUCache(config.DedupeSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create dedupe cache: %w", err)
	}

	return &engine{
		config:    config,
		clipboard: config.Clipboard,
		topology:  config.Topology,
		logger:    config.Logger,
		dedupe:    dedupe,
		stats: Stats{
			StartTime: time.Now(),
		},
	}, nil
}

// Run starts the sync engine main loop and blocks until the context is cancelled.
// This method orchestrates all synchronization activities:
//
//  1. Initializes the engine with current clipboard state
//  2. Starts watching for local clipboard changes
//  3. Listens for messages from peer nodes
//  4. Monitors topology changes
//  5. Processes all events in a coordinated manner
//
// The engine will run continuously until the context is cancelled or an
// unrecoverable error occurs. It handles temporary failures gracefully and
// attempts to maintain synchronization even during network disruptions.
//
// Context cancellation triggers a graceful shutdown, ensuring all resources
// are properly cleaned up.
func (e *engine) Run(ctx context.Context) error {
	e.logger.Info("sync engine starting", "node", e.config.NodeID)

	// Initialize with current clipboard content
	if err := e.initializeState(); err != nil {
		return fmt.Errorf("failed to initialize state: %w", err)
	}

	// Watch local clipboard
	clipCh := e.clipboard.Watch(ctx)

	// Watch topology events
	eventCh := e.topology.Events()

	// Watch incoming messages
	msgCh := e.topology.Messages()

	// Main event loop
	for {
		select {
		case _, ok := <-clipCh:
			if !ok {
				return fmt.Errorf("clipboard watch channel closed")
			}
			// Notification received - pull current state
			content, err := e.clipboard.Read()
			if err != nil {
				e.logger.Error("failed to read clipboard", "error", err)
				continue
			}
			state := e.clipboard.GetState()
			e.handleLocalChange(content, state)

		case msg, ok := <-msgCh:
			if !ok {
				return fmt.Errorf("message channel closed")
			}
			e.handleIncomingMessage(msg)

		case event, ok := <-eventCh:
			if !ok {
				e.logger.Debug("event channel closed")
				continue
			}
			e.handleTopologyEvent(event)

		case <-ctx.Done():
			e.logger.Info("sync engine stopping", "node", e.config.NodeID)
			return ctx.Err()
		}
	}
}

// Stats returns current engine statistics.
func (e *engine) Stats() *Stats {
	e.mu.RLock()
	lastLocalChange := e.lastLocalChange
	lastRemoteChange := e.lastRemoteChange
	e.mu.RUnlock()

	// Create a copy with atomic reads
	return &Stats{
		MessagesSent:      atomic.LoadUint64(&e.stats.MessagesSent),
		MessagesReceived:  atomic.LoadUint64(&e.stats.MessagesReceived),
		MessagesDuplicate: atomic.LoadUint64(&e.stats.MessagesDuplicate),
		LocalChanges:      atomic.LoadUint64(&e.stats.LocalChanges),
		RemoteChanges:     atomic.LoadUint64(&e.stats.RemoteChanges),
		ConflictsWon:      atomic.LoadUint64(&e.stats.ConflictsWon),
		ConflictsLost:     atomic.LoadUint64(&e.stats.ConflictsLost),
		SendErrors:        atomic.LoadUint64(&e.stats.SendErrors),
		ReceiveErrors:     atomic.LoadUint64(&e.stats.ReceiveErrors),
		LastLocalChange:   lastLocalChange,
		LastRemoteChange:  lastRemoteChange,
		StartTime:         e.stats.StartTime,
	}
}

// initializeState reads current clipboard state.
func (e *engine) initializeState() error {
	content, err := e.clipboard.Read()
	if err != nil {
		e.logger.Error("failed to read initial clipboard", "error", err)
		// Non-fatal: start with empty clipboard
		content = ""
	}

	e.mu.Lock()
	e.current = content
	e.checksum = computeChecksum(content)
	e.timestamp = time.Now().UnixNano()
	e.mu.Unlock()

	// Add to dedupe cache
	e.dedupe.Add(e.checksum, e.timestamp)

	e.logger.Info("initialized clipboard state",
		"content_length", len(content),
		"checksum", e.checksum[:8], // First 8 chars for logging
	)

	return nil
}

// handleLocalChange processes local clipboard changes detected by the OS monitor.
// This is triggered when the user copies new content to their clipboard.
//
// Processing steps:
//  1. Check if content actually changed using state hash
//  2. Detect potential sync loops using timing heuristics
//  3. Update internal state with new content
//  4. Add to deduplication cache
//  5. Broadcast change to all connected peers
//
// The state parameter provides sequence number and hash information to help
// detect duplicate notifications that may occur with rapid clipboard changes.
//
// Sync loop detection prevents infinite loops when our own changes echo back
// through the network. This uses timing windows to identify suspiciously fast
// round-trip updates.
func (e *engine) handleLocalChange(content string, state clipboard.ClipboardState) {
	checksum := computeChecksum(content)

	// Verify state hash matches content (defensive check)
	if state.ContentHash != "" && state.ContentHash != checksum {
		e.logger.Error("clipboard state hash mismatch",
			"state_hash", state.ContentHash[:8],
			"content_hash", checksum[:8],
		)
	}

	e.mu.Lock()
	// Check if content actually changed
	if checksum == e.checksum {
		e.mu.Unlock()
		e.logger.Debug("clipboard unchanged, ignoring")
		return
	}

	// Check for sync loop
	if e.isSyncLoop() {
		e.mu.Unlock()
		e.logger.Debug("potential sync loop detected, ignoring change")
		atomic.AddUint64(&e.stats.MessagesDuplicate, 1)
		return
	}

	// Update state
	e.current = content
	e.checksum = checksum
	e.timestamp = time.Now().UnixNano()
	e.lastLocalChange = time.Now()
	e.mu.Unlock()

	// Add to dedupe cache
	e.dedupe.Add(checksum, e.timestamp)

	// Check size limits before broadcasting
	if len(content) > e.config.MaxClipboardSize {
		e.logger.Error("clipboard content exceeds maximum size",
			"size", len(content),
			"limit", e.config.MaxClipboardSize,
		)
		atomic.AddUint64(&e.stats.SendErrors, 1)
		return
	}

	// Create and broadcast message
	clipboardMsg := NewClipboardMessage(e.config.NodeID, content)

	// Marshal the clipboard message
	data, err := clipboardMsg.Marshal()
	if err != nil {
		e.logger.Error("failed to marshal clipboard message", "error", err)
		atomic.AddUint64(&e.stats.SendErrors, 1)
		return
	}

	// Create type-safe mesh message
	meshMsg := mesh.NewClipboardMessage(e.config.NodeID, json.RawMessage(data))

	e.logger.Info("broadcasting local clipboard change",
		"content_length", len(content),
		"checksum", checksum[:8],
	)

	if err := e.topology.Broadcast(meshMsg); err != nil {
		e.logger.Error("failed to broadcast clipboard change", "error", err)
		atomic.AddUint64(&e.stats.SendErrors, 1)
	} else {
		atomic.AddUint64(&e.stats.MessagesSent, 1)
		atomic.AddUint64(&e.stats.LocalChanges, 1)
	}
}

// handleIncomingMessage processes clipboard synchronization messages from peer nodes.
// This is the core of the distributed synchronization protocol.
//
// Message processing pipeline:
//  1. Filter for clipboard messages (ignore other types)
//  2. Parse and validate message structure
//  3. Verify checksum matches content
//  4. Check deduplication cache for seen messages
//  5. Apply conflict resolution logic
//  6. Update local clipboard if remote wins
//
// The method handles various message formats for compatibility and includes
// extensive error handling to ensure malformed messages don't disrupt the
// synchronization process.
func (e *engine) handleIncomingMessage(msg mesh.Message) {
	atomic.AddUint64(&e.stats.MessagesReceived, 1)

	// Only handle clipboard messages
	if msg.Type != string(MessageTypeClipboard) {
		e.logger.Debug("ignoring non-clipboard message", "type", msg.Type)
		return
	}

	// Parse message - handle both []byte and json.RawMessage
	var payloadBytes []byte
	switch p := msg.Payload.(type) {
	case []byte:
		payloadBytes = p
	case json.RawMessage:
		payloadBytes = []byte(p)
	default:
		e.logger.Error("invalid payload type", "type", fmt.Sprintf("%T", msg.Payload))
		atomic.AddUint64(&e.stats.ReceiveErrors, 1)
		return
	}

	clipMsg, err := UnmarshalClipboardMessage(payloadBytes)
	if err != nil {
		e.logger.Error("failed to unmarshal clipboard message", "error", err)
		atomic.AddUint64(&e.stats.ReceiveErrors, 1)
		return
	}

	// Validate message
	if err := clipMsg.Validate(); err != nil {
		e.logger.Error("invalid clipboard message", "error", err, "from", msg.From)
		atomic.AddUint64(&e.stats.ReceiveErrors, 1)
		return
	}

	// Check size limits
	if len(clipMsg.Content) > e.config.MaxClipboardSize {
		e.logger.Error("received clipboard content exceeds maximum size",
			"size", len(clipMsg.Content),
			"limit", e.config.MaxClipboardSize,
			"from", msg.From,
		)
		atomic.AddUint64(&e.stats.ReceiveErrors, 1)
		return
	}

	// Check deduplication
	if e.isDuplicate(clipMsg) {
		e.logger.Debug("duplicate message, ignoring",
			"from", clipMsg.NodeID,
			"checksum", clipMsg.Checksum[:8],
		)
		atomic.AddUint64(&e.stats.MessagesDuplicate, 1)
		return
	}

	// Apply conflict resolution
	if e.shouldApplyRemoteChange(clipMsg) {
		e.applyRemoteChange(clipMsg)
	}
}

// isDuplicate checks if we've seen this message before.
func (e *engine) isDuplicate(msg *ClipboardMessage) bool {
	if timestamp, exists := e.dedupe.Get(msg.Checksum); exists {
		// If we've seen this checksum with a recent timestamp, it's a duplicate
		if timestampNanos, ok := timestamp.(int64); ok {
			timeDiff := time.Duration(msg.Timestamp - timestampNanos)
			if timeDiff.Abs() < e.config.SyncLoopWindow {
				return true
			}
		}
	}
	return false
}

// shouldApplyRemoteChange implements the conflict resolution algorithm.
// This determines whether a remote clipboard change should overwrite local state.
//
// Conflict Resolution Strategy (Last-Write-Wins):
//  1. Compare timestamps - newer timestamp wins
//  2. If timestamps are equal (rare), use node ID as tiebreaker
//  3. Higher node ID wins in case of ties
//
// This approach ensures:
//   - Deterministic resolution (all nodes reach same decision)
//   - No coordination required between nodes
//   - Eventually consistent state across the mesh
//
// The method is called with read lock held on engine state.
func (e *engine) shouldApplyRemoteChange(msg *ClipboardMessage) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// If remote timestamp is newer, apply it
	if msg.Timestamp > e.timestamp {
		return true
	}

	// If timestamps are equal (unlikely), use node ID as tiebreaker
	if msg.Timestamp == e.timestamp && msg.NodeID > e.config.NodeID {
		return true
	}

	e.logger.Debug("remote change is older, ignoring",
		"remote_time", time.Unix(0, msg.Timestamp),
		"local_time", time.Unix(0, e.timestamp),
	)

	return false
}

// applyRemoteChange updates local clipboard with remote content.
func (e *engine) applyRemoteChange(msg *ClipboardMessage) {
	e.mu.Lock()
	wasNewer := msg.Timestamp > e.timestamp
	e.current = msg.Content
	e.checksum = msg.Checksum
	e.timestamp = msg.Timestamp
	e.lastRemoteChange = time.Now()
	e.mu.Unlock()

	// Add to dedupe cache
	e.dedupe.Add(msg.Checksum, msg.Timestamp)

	// Update clipboard
	if err := e.clipboard.Write(msg.Content); err != nil {
		e.logger.Error("failed to write clipboard", "error", err)
		atomic.AddUint64(&e.stats.ReceiveErrors, 1)
		return
	}

	e.logger.Info("applied remote clipboard change",
		"from", msg.NodeID,
		"content_length", len(msg.Content),
		"checksum", msg.Checksum[:8],
	)

	atomic.AddUint64(&e.stats.RemoteChanges, 1)
	if wasNewer {
		atomic.AddUint64(&e.stats.ConflictsLost, 1)
	} else {
		atomic.AddUint64(&e.stats.ConflictsWon, 1)
	}
}

// isSyncLoop detects potential synchronization loops that can occur when
// our own changes echo back through the network.
//
// Detection mechanism:
//   - Tracks timing of recent remote changes
//   - If local change occurs very soon after remote change, it's likely an echo
//   - Uses configurable time window (MinChangeInterval)
//
// This heuristic prevents common scenarios like:
//  1. Node A changes clipboard
//  2. Node B receives and applies change
//  3. Node B's clipboard watcher fires (detecting "new" content)
//  4. Node B broadcasts "change" back to Node A
//
// Without this protection, the network could enter an infinite sync loop.
func (e *engine) isSyncLoop() bool {
	now := time.Now()

	// If we recently applied a remote change, and now see a local change,
	// it might be our own remote change echoing back
	if !e.lastRemoteChange.IsZero() &&
		now.Sub(e.lastRemoteChange) < e.config.MinChangeInterval {
		return true
	}

	return false
}

// handleTopologyEvent processes topology changes.
func (e *engine) handleTopologyEvent(event mesh.TopologyEvent) {
	switch event.Type {
	case mesh.PeerConnected:
		e.logger.Info("peer connected", "peer", event.Peer.ID)
		// Could send current state to new peer, but for simplicity
		// we'll let them request it if needed

	case mesh.PeerDisconnected:
		e.logger.Info("peer disconnected", "peer", event.Peer.ID)
		// No action needed for disconnections
	}
}
