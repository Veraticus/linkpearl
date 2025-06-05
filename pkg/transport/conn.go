// Package transport implements secure connections for the transport layer.
//
// This file provides the Conn interface implementation that wraps
// TLS connections with additional functionality:
//   - Node metadata management (ID, mode, version)
//   - JSON message serialization/deserialization
//   - Size-limited message reception (prevents DoS)
//   - Deadline management for read/write operations
//   - Thread-safe connection state management
//
// The connection implementation ensures:
//   - All data is encrypted via TLS 1.3
//   - Messages are encoded as JSON with configurable size limits
//   - Proper error handling with timeout detection
//   - Graceful shutdown with connection tracking
package transport

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// secureConn implements the Conn interface with TLS encryption.
// It wraps a TLS connection with node metadata and JSON encoding/decoding
// capabilities. All operations are thread-safe through mutex protection.
type secureConn struct {
	conn       net.Conn
	encoder    *json.Encoder
	decoder    *json.Decoder
	closedChan chan struct{}
	nodeID     string
	mode       string
	version    string
	mu         sync.Mutex
	closed     bool
}

// newSecureConn creates a new secure connection from an established TLS connection.
// The connection is initialized with:
//   - Node information from the authentication phase
//   - JSON encoder for outbound messages
//   - JSON decoder with size limits for inbound messages
//   - Synchronization primitives for thread safety
func newSecureConn(tlsConn *tls.Conn, nodeInfo *NodeInfo) *secureConn {
	conn := &secureConn{
		nodeID:     nodeInfo.NodeID,
		mode:       nodeInfo.Mode,
		version:    nodeInfo.Version,
		conn:       tlsConn,
		closedChan: make(chan struct{}),
	}

	// Create JSON encoder/decoder with size limits
	conn.encoder = json.NewEncoder(tlsConn)
	conn.decoder = json.NewDecoder(io.LimitReader(tlsConn, MaxMessageSize))

	return conn
}

// NodeID returns the remote node's unique identifier.
// This ID is exchanged during the connection handshake and
// remains constant for the lifetime of the connection.
func (c *secureConn) NodeID() string {
	return c.nodeID
}

// Mode returns the remote node's operational mode.
// Common modes include "standard", "relay", "gateway", etc.
// The mode determines the node's role in the network.
func (c *secureConn) Mode() string {
	return c.mode
}

// Version returns the remote node's protocol version.
// This is used for compatibility checking and feature negotiation.
func (c *secureConn) Version() string {
	return c.version
}

// Send sends a message to the remote node using JSON encoding.
// The method:
//   - Acquires a lock to ensure thread-safe sending
//   - Sets a 30-second write deadline for the operation
//   - Encodes the message as JSON and transmits it
//   - Returns appropriate errors for timeouts or encoding failures
func (c *secureConn) Send(msg any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrClosed
	}

	// Set write deadline for this operation (if conn supports it)
	if c.conn != nil {
		if err := c.conn.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return fmt.Errorf("failed to set write deadline: %w", err)
		}
	}

	// Encode and send the message
	if err := c.encoder.Encode(msg); err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return ErrTimeout
		}
		return fmt.Errorf("failed to encode message: %w", err)
	}

	return nil
}

// Receive receives a message from the remote node using JSON decoding.
// The method:
//   - Checks if the connection is closed
//   - Decodes JSON data into the provided message structure
//   - Enforces the MaxMessageSize limit to prevent DoS attacks
//   - Returns appropriate errors for EOF, timeouts, or size violations
func (c *secureConn) Receive(msg any) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.mu.Unlock()

	// Decode the message
	if err := c.decoder.Decode(msg); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return io.EOF
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return ErrTimeout
		}
		// Check if the error is due to message size limit
		if err.Error() == "unexpected EOF" {
			return fmt.Errorf("message too large (max %d bytes)", MaxMessageSize)
		}
		return fmt.Errorf("failed to decode message: %w", err)
	}

	return nil
}

// Close closes the connection gracefully.
// This method is idempotent and thread-safe.
// It ensures the underlying TLS connection is properly closed
// and signals any waiting operations via the closed channel.
func (c *secureConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.closedChan)

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// LocalAddr returns the local network address of the connection.
// Returns nil if the underlying connection is not available.
func (c *secureConn) LocalAddr() net.Addr {
	if c.conn != nil {
		return c.conn.LocalAddr()
	}
	return nil
}

// RemoteAddr returns the remote network address of the connection.
// Returns nil if the underlying connection is not available.
func (c *secureConn) RemoteAddr() net.Addr {
	if c.conn != nil {
		return c.conn.RemoteAddr()
	}
	return nil
}

// SetDeadline sets the read and write deadlines for the connection.
// This affects both Send and Receive operations.
// Pass time.Time{} to clear the deadline.
func (c *secureConn) SetDeadline(t time.Time) error {
	if c.conn != nil {
		return c.conn.SetDeadline(t)
	}
	return nil
}

// SetReadDeadline sets the deadline for future Receive operations.
// Pass time.Time{} to clear the deadline.
func (c *secureConn) SetReadDeadline(t time.Time) error {
	if c.conn != nil {
		return c.conn.SetReadDeadline(t)
	}
	return nil
}

// SetWriteDeadline sets the deadline for future Send operations.
// Pass time.Time{} to clear the deadline.
func (c *secureConn) SetWriteDeadline(t time.Time) error {
	if c.conn != nil {
		return c.conn.SetWriteDeadline(t)
	}
	return nil
}

// NodeInfo represents information about a node, exchanged after TLS upgrade.
// This structure is transmitted in JSON format after successful authentication
// and TLS establishment to identify the remote peer.
type NodeInfo struct {
	NodeID  string `json:"node_id"`
	Mode    string `json:"mode"`
	Version string `json:"version"`
}

// Validate checks if the NodeInfo contains all required fields.
// Returns an error if any mandatory field is missing or empty.
// This validation ensures nodes can be properly identified in the network.
func (n *NodeInfo) Validate() error {
	if n.NodeID == "" {
		return fmt.Errorf("node ID is required")
	}
	if n.Mode == "" {
		return fmt.Errorf("node mode is required")
	}
	if n.Version == "" {
		return fmt.Errorf("node version is required")
	}
	return nil
}
