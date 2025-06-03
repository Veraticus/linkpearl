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

// Transport defines the interface for secure network communication
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

// Conn represents a secure, authenticated connection
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

// Message is a generic message type for transport
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

// TransportConfig holds configuration for creating a transport
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

// Logger interface for transport logging
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
