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

// secureConn implements the Conn interface with TLS encryption
type secureConn struct {
	// Connection metadata
	nodeID  string
	mode    string
	version string

	// Underlying connection (interface to allow testing)
	conn net.Conn

	// JSON encoder/decoder for message serialization
	encoder *json.Encoder
	decoder *json.Decoder

	// Synchronization
	mu         sync.Mutex
	closed     bool
	closedChan chan struct{}
}

// newSecureConn creates a new secure connection from an established TLS connection
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

// NodeID returns the remote node's ID
func (c *secureConn) NodeID() string {
	return c.nodeID
}

// Mode returns the remote node's mode
func (c *secureConn) Mode() string {
	return c.mode
}

// Version returns the remote node's version
func (c *secureConn) Version() string {
	return c.version
}

// Send sends a message to the remote node
func (c *secureConn) Send(msg interface{}) error {
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

// Receive receives a message from the remote node
func (c *secureConn) Receive(msg interface{}) error {
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

// Close closes the connection
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

// LocalAddr returns the local network address
func (c *secureConn) LocalAddr() net.Addr {
	if c.conn != nil {
		return c.conn.LocalAddr()
	}
	return nil
}

// RemoteAddr returns the remote network address
func (c *secureConn) RemoteAddr() net.Addr {
	if c.conn != nil {
		return c.conn.RemoteAddr()
	}
	return nil
}

// SetDeadline sets the read and write deadlines
func (c *secureConn) SetDeadline(t time.Time) error {
	if c.conn != nil {
		return c.conn.SetDeadline(t)
	}
	return nil
}

// SetReadDeadline sets the read deadline
func (c *secureConn) SetReadDeadline(t time.Time) error {
	if c.conn != nil {
		return c.conn.SetReadDeadline(t)
	}
	return nil
}

// SetWriteDeadline sets the write deadline
func (c *secureConn) SetWriteDeadline(t time.Time) error {
	if c.conn != nil {
		return c.conn.SetWriteDeadline(t)
	}
	return nil
}

// NodeInfo represents information about a node, exchanged after TLS upgrade
type NodeInfo struct {
	NodeID  string `json:"node_id"`
	Mode    string `json:"mode"`
	Version string `json:"version"`
}

// Validate checks if the NodeInfo is valid
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
