// Package transport provides a secure networking layer for LinkPearl nodes.
//
// The transport package implements a comprehensive networking solution that handles:
//   - Secure peer-to-peer communication between LinkPearl nodes
//   - Mutual authentication using shared secrets and HMAC-based handshakes
//   - Automatic TLS encryption with ephemeral certificates
//   - Node identification and version exchange
//   - Message serialization and deserialization
//   - Connection lifecycle management
//
// # Architecture Overview
//
// The transport layer is built on a modular design with clear separation of concerns:
//
//   - Transport interface: Manages listening, accepting, and establishing connections
//   - Conn interface: Represents authenticated, encrypted connections between nodes
//   - Authenticator: Handles the security handshake and TLS setup
//   - Message passing: JSON-based protocol for inter-node communication
//
// # Security Model
//
// Security is implemented in multiple layers:
//
//  1. Authentication: Nodes authenticate using a shared secret and HMAC-based
//     challenge-response protocol with time-bound nonces
//  2. Encryption: All data is encrypted using TLS 1.3 with strong cipher suites
//  3. Certificate Management: Ephemeral certificates are generated per connection
//  4. Message Integrity: JSON encoding with size limits prevents malformed data
//
// # Usage Example
//
//	config := &TransportConfig{
//	    NodeID: "node-123",
//	    Mode:   "standard",
//	    Secret: "shared-secret-key",
//	    Logger: myLogger,
//	}
//
//	transport := NewTCPTransport(config)
//
//	// Server mode
//	err := transport.Listen(":8080")
//	conn, err := transport.Accept()
//
//	// Client mode
//	conn, err := transport.Connect(ctx, "localhost:8080")
//
//	// Send/receive messages
//	err = conn.Send(myMessage)
//	err = conn.Receive(&receivedMessage)
//
package transport

import (
	"context"
	"errors"
	"net"
	"time"
)

// Common errors
var (
	// ErrAuthFailed indicates authentication failure
	ErrAuthFailed = errors.New("authentication failed")

	// ErrTimeout indicates a timeout occurred
	ErrTimeout = errors.New("operation timed out")

	// ErrClosed indicates the transport is closed
	ErrClosed = errors.New("transport closed")

	// ErrInvalidMessage indicates a malformed message
	ErrInvalidMessage = errors.New("invalid message format")
)

// Version is the current protocol version
const Version = "1.0.0"

// Transport defines the interface for secure network communication.
// Implementations of this interface provide the core networking functionality
// for LinkPearl nodes, including connection management, authentication,
// and encrypted communication channels.
type Transport interface {
	// Listen starts accepting connections (server mode)
	Listen(addr string) error

	// Connect establishes an outbound connection
	Connect(ctx context.Context, addr string) (Conn, error)

	// Accept returns the next incoming connection
	Accept() (Conn, error)

	// Close shuts down the transport
	Close() error

	// Addr returns the listener address (nil if not listening)
	Addr() net.Addr
}

// Conn represents a secure, authenticated connection between LinkPearl nodes.
// Each connection is:
//   - Mutually authenticated using the shared secret protocol
//   - Encrypted using TLS 1.3 with ephemeral certificates
//   - Identified by the remote node's ID, mode, and version
//   - Capable of bidirectional message exchange with JSON serialization
type Conn interface {
	// NodeID returns the remote node's ID
	NodeID() string

	// Mode returns the remote node's mode
	Mode() string

	// Version returns the remote node's version
	Version() string

	// Send sends a message to the remote node
	Send(msg interface{}) error

	// Receive receives a message from the remote node
	Receive(msg interface{}) error

	// Close closes the connection
	Close() error

	// LocalAddr returns the local network address
	LocalAddr() net.Addr

	// RemoteAddr returns the remote network address
	RemoteAddr() net.Addr

	// SetDeadline sets the read and write deadlines
	SetDeadline(t time.Time) error

	// SetReadDeadline sets the read deadline
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline sets the write deadline
	SetWriteDeadline(t time.Time) error
}

// Message is a generic message type for transport communication.
// All messages exchanged between nodes should implement this interface
// to ensure proper type identification and routing.
type Message interface {
	// Type returns the message type identifier
	Type() string
}

// HandshakeTimeout is the maximum time allowed for authentication handshake
const HandshakeTimeout = 10 * time.Second

// KeepAliveInterval is the interval between keepalive messages
const KeepAliveInterval = 30 * time.Second

// MaxMessageSize is the maximum allowed message size (1MB)
const MaxMessageSize = 1024 * 1024

// TransportConfig holds configuration for creating a transport instance.
// This configuration defines the node's identity, operational mode,
// authentication credentials, and optional TLS settings.
type TransportConfig struct {
	// NodeID is this node's unique identifier
	NodeID string

	// Mode is this node's operational mode
	Mode string

	// Secret is the shared secret for authentication
	Secret string

	// TLSConfig provides optional TLS configuration
	// If nil, ephemeral certificates will be generated
	TLSConfig interface{}

	// Logger provides optional logging interface
	Logger Logger
}

// Logger interface for transport logging.
// Implementations should provide structured logging with support
// for different log levels (Debug, Info, Error) and key-value pairs.
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// noopLogger implements Logger with no-op methods
type noopLogger struct{}

func (noopLogger) Debug(msg string, args ...interface{}) {}
func (noopLogger) Info(msg string, args ...interface{})  {}
func (noopLogger) Error(msg string, args ...interface{}) {}

// DefaultLogger returns a no-op logger
func DefaultLogger() Logger {
	return noopLogger{}
}
