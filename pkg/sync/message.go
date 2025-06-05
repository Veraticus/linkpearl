// message.go defines the wire protocol for clipboard synchronization messages.
// These messages are exchanged between nodes to maintain clipboard consistency
// across the mesh network.
//
// Message Format:
//
// ClipboardMessage is the primary message type containing:
//   - NodeID: Identifies the originating node
//   - Timestamp: UnixNano timestamp for conflict resolution
//   - Checksum: SHA256 hash of content for integrity verification
//   - Content: The actual clipboard data
//   - Version: Protocol version for future compatibility
//
// Security Considerations:
//
// Messages include checksums to ensure data integrity during transmission.
// The checksum is computed using SHA256, providing both integrity verification
// and deduplication capabilities. Any tampering or corruption will be detected
// when the computed checksum doesn't match the transmitted one.
//
// Conflict Resolution:
//
// The timestamp field enables last-write-wins conflict resolution. When two
// nodes have conflicting clipboard states, the one with the more recent
// timestamp wins. Using UnixNano provides microsecond precision, making
// timestamp collisions extremely rare.
//
// Wire Format:
//
// Messages are serialized as JSON for simplicity and debuggability. While
// this adds some overhead compared to binary protocols, it simplifies
// debugging and ensures compatibility across different platforms.

package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// MessageType identifies the type of sync message.
// Currently only clipboard synchronization is supported, but the type
// field allows for future protocol extensions.
type MessageType string

const (
	// MessageTypeClipboard is for clipboard synchronization
	MessageTypeClipboard MessageType = "clipboard"
)

// ClipboardMessage represents a clipboard synchronization message exchanged
// between nodes in the mesh network. This is the primary data structure for
// the synchronization protocol.
//
// Fields:
//   - NodeID: Unique identifier of the originating node
//   - Timestamp: UnixNano timestamp for ordering and conflict resolution
//   - Checksum: SHA256 hash for integrity verification and deduplication
//   - Content: The actual clipboard data being synchronized
//   - Version: Protocol version for backward compatibility
//
// The message is designed to be self-contained with all information needed
// for conflict resolution and integrity verification.
type ClipboardMessage struct {
	NodeID    string `json:"node_id"`
	Timestamp int64  `json:"timestamp"` // UnixNano for precision
	Checksum  string `json:"checksum"`  // SHA256 of content
	Content   string `json:"content"`
	Version   string `json:"version"` // For future compatibility
}

// NewClipboardMessage creates a new clipboard message with current timestamp.
// This constructor ensures all required fields are properly initialized:
//   - Automatically generates current timestamp in nanoseconds
//   - Computes SHA256 checksum of the content
//   - Sets protocol version for compatibility
//
// The timestamp precision (nanoseconds) minimizes the chance of collisions
// when multiple nodes change their clipboards simultaneously.
func NewClipboardMessage(nodeID, content string) *ClipboardMessage {
	return &ClipboardMessage{
		NodeID:    nodeID,
		Timestamp: time.Now().UnixNano(),
		Checksum:  computeChecksum(content),
		Content:   content,
		Version:   "1.0",
	}
}

// computeChecksum calculates SHA256 checksum of content.
// This serves multiple purposes:
//  1. Integrity verification - detects corruption during transmission
//  2. Deduplication - identical content produces identical checksums
//  3. Change detection - different content always has different checksums
//
// SHA256 is chosen for its strong collision resistance and widespread
// availability across platforms.
func computeChecksum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// Marshal converts the message to JSON bytes
func (m *ClipboardMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalClipboardMessage decodes a clipboard message from JSON
func UnmarshalClipboardMessage(data []byte) (*ClipboardMessage, error) {
	var msg ClipboardMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// Validate checks if the message is valid and internally consistent.
// This method performs several critical checks:
//  1. All required fields are present and non-empty
//  2. Timestamp is positive (valid Unix time)
//  3. Checksum matches the actual content
//
// The checksum verification is particularly important as it ensures the
// message hasn't been corrupted or tampered with during transmission.
// Any validation failure indicates the message should be discarded.
//
// Returns specific errors (ErrInvalidMessage, ErrChecksumMismatch) to
// help with debugging and monitoring.
func (m *ClipboardMessage) Validate() error {
	if m.NodeID == "" {
		return ErrInvalidMessage
	}
	if m.Timestamp <= 0 {
		return ErrInvalidMessage
	}
	if m.Checksum == "" {
		return ErrInvalidMessage
	}
	// Verify checksum matches content
	expectedChecksum := computeChecksum(m.Content)
	if m.Checksum != expectedChecksum {
		return ErrChecksumMismatch
	}
	return nil
}

// Time returns the message timestamp as time.Time for easier manipulation
// and comparison. This converts from UnixNano format to Go's time.Time.
//
// Useful for:
//   - Logging and debugging (human-readable timestamps)
//   - Time-based calculations and comparisons
//   - Integration with other time-based systems
func (m *ClipboardMessage) Time() time.Time {
	return time.Unix(0, m.Timestamp)
}
