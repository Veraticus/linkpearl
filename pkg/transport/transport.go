// transport.go implements the TCP-based transport layer for LinkPearl.
//
// This file contains the main transport implementation that handles:
//   - TCP socket management and connection lifecycle
//   - Server mode with listening and accepting connections
//   - Client mode with outbound connection establishment  
//   - Integration with the authentication and TLS layers
//   - Node information exchange after secure channel setup
//   - Connection tracking and graceful shutdown
//
// The transport follows a specific connection establishment flow:
//  1. TCP connection establishment
//  2. Authentication handshake (HMAC-based shared secret)
//  3. TLS upgrade with ephemeral certificates
//  4. Node information exchange (ID, mode, version)
//  5. Ready for application-level messaging
//
package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// tcpTransport implements the Transport interface using TCP sockets.
// It manages both server and client operations, maintaining a registry
// of active connections and ensuring proper cleanup on shutdown.
type tcpTransport struct {
	config   *TransportConfig
	auth     Authenticator
	listener net.Listener
	logger   Logger

	mu     sync.RWMutex
	closed bool
	conns  map[string]*secureConn
}

// NewTCPTransport creates a new TCP transport instance.
// The provided configuration must include:
//   - NodeID: unique identifier for this node
//   - Mode: operational mode (e.g., "standard", "relay")
//   - Secret: shared secret for authentication
//   - Logger: optional logger (defaults to no-op if nil)
func NewTCPTransport(config *TransportConfig) Transport {
	if config.Logger == nil {
		config.Logger = DefaultLogger()
	}

	return &tcpTransport{
		config: config,
		auth:   NewAuthenticator(config.Logger),
		logger: config.Logger,
		conns:  make(map[string]*secureConn),
	}
}

// Listen starts accepting connections on the specified address (server mode).
// The address format follows Go's net.Listen conventions (e.g., ":8080", "0.0.0.0:8080").
// Only one listener can be active per transport instance.
func (t *tcpTransport) Listen(addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	if t.listener != nil {
		return fmt.Errorf("transport already listening")
	}

	// Start TCP listener
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	t.listener = listener
	t.logger.Info("transport listening", "addr", listener.Addr())

	return nil
}

// Accept returns the next incoming connection from the listener.
// This method blocks until a connection is available or an error occurs.
// The returned connection is fully authenticated and encrypted.
// 
// The acceptance process includes:
//   - TCP connection acceptance with keepalive settings
//   - Authentication handshake as server
//   - TLS upgrade with server-side certificate
//   - Node information exchange
func (t *tcpTransport) Accept() (Conn, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil, ErrClosed
	}
	listener := t.listener
	t.mu.RUnlock()

	if listener == nil {
		return nil, fmt.Errorf("transport not listening")
	}

	// Accept TCP connection
	tcpConn, err := listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("failed to accept connection: %w", err)
	}

	// Configure TCP keepalive
	if tc, ok := tcpConn.(*net.TCPConn); ok {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(KeepAliveInterval)
	}

	t.logger.Debug("accepted connection", "remote", tcpConn.RemoteAddr())

	// Perform authentication handshake
	authResult, err := t.auth.Handshake(tcpConn, t.config.Secret, true)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	if !authResult.Success {
		tcpConn.Close()
		return nil, ErrAuthFailed
	}

	// Generate TLS config
	tlsConfig, err := t.auth.GenerateTLSConfig(true)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("failed to generate TLS config: %w", err)
	}

	// Upgrade to TLS
	tlsConn := tls.Server(tcpConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	// Exchange node information
	nodeInfo, err := t.exchangeNodeInfo(tlsConn, true)
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("failed to exchange node info: %w", err)
	}

	// Create secure connection
	conn := newSecureConn(tlsConn, nodeInfo)

	// Track connection
	t.mu.Lock()
	t.conns[nodeInfo.NodeID] = conn
	t.mu.Unlock()

	t.logger.Info("connection established", "nodeID", nodeInfo.NodeID, "mode", nodeInfo.Mode)

	return conn, nil
}

// Connect establishes an outbound connection to the specified address.
// The context can be used to cancel the connection attempt.
// Returns a fully authenticated and encrypted connection.
//
// The connection process includes:
//   - TCP dial with timeout and keepalive
//   - Authentication handshake as client
//   - TLS upgrade with client-side certificate  
//   - Node information exchange
func (t *tcpTransport) Connect(ctx context.Context, addr string) (Conn, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil, ErrClosed
	}
	t.mu.RUnlock()

	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout:   HandshakeTimeout,
		KeepAlive: KeepAliveInterval,
	}

	// Dial TCP connection
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	t.logger.Debug("connected to", "remote", tcpConn.RemoteAddr())

	// Perform authentication handshake
	authResult, err := t.auth.Handshake(tcpConn, t.config.Secret, false)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	if !authResult.Success {
		tcpConn.Close()
		return nil, ErrAuthFailed
	}

	// Generate TLS config
	tlsConfig, err := t.auth.GenerateTLSConfig(false)
	if err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("failed to generate TLS config: %w", err)
	}

	// Upgrade to TLS
	tlsConn := tls.Client(tcpConn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	// Exchange node information
	nodeInfo, err := t.exchangeNodeInfo(tlsConn, false)
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("failed to exchange node info: %w", err)
	}

	// Create secure connection
	conn := newSecureConn(tlsConn, nodeInfo)

	// Track connection
	t.mu.Lock()
	t.conns[nodeInfo.NodeID] = conn
	t.mu.Unlock()

	t.logger.Info("connection established", "nodeID", nodeInfo.NodeID, "mode", nodeInfo.Mode)

	return conn, nil
}

// exchangeNodeInfo exchanges node information after TLS upgrade.
// This method implements a coordinated exchange where:
//   - Server receives first, then sends
//   - Client sends first, then receives
// This ensures both sides have each other's information without deadlock.
func (t *tcpTransport) exchangeNodeInfo(conn *tls.Conn, isServer bool) (*NodeInfo, error) {
	// Create our node info
	ourInfo := &NodeInfo{
		NodeID:  t.config.NodeID,
		Mode:    t.config.Mode,
		Version: Version,
	}

	// Set deadline for exchange
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, fmt.Errorf("failed to set deadline: %w", err)
	}
	defer conn.SetDeadline(time.Time{})

	// Create encoder/decoder
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// Exchange info (server receives first, client sends first)
	var remoteInfo NodeInfo

	if isServer {
		// Server: receive then send
		if err := decoder.Decode(&remoteInfo); err != nil {
			return nil, fmt.Errorf("failed to receive node info: %w", err)
		}
		if err := encoder.Encode(ourInfo); err != nil {
			return nil, fmt.Errorf("failed to send node info: %w", err)
		}
	} else {
		// Client: send then receive
		if err := encoder.Encode(ourInfo); err != nil {
			return nil, fmt.Errorf("failed to send node info: %w", err)
		}
		if err := decoder.Decode(&remoteInfo); err != nil {
			return nil, fmt.Errorf("failed to receive node info: %w", err)
		}
	}

	// Validate remote info
	if err := remoteInfo.Validate(); err != nil {
		return nil, fmt.Errorf("invalid node info: %w", err)
	}

	return &remoteInfo, nil
}

// Close shuts down the transport gracefully.
// This method:
//   - Marks the transport as closed to prevent new operations
//   - Closes all active connections
//   - Stops the listener if running
//   - Returns any error from closing the listener
func (t *tcpTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true

	// Close all connections
	for _, conn := range t.conns {
		conn.Close()
	}
	t.conns = make(map[string]*secureConn)

	// Close listener if active
	if t.listener != nil {
		err := t.listener.Close()
		t.listener = nil
		return err
	}

	return nil
}

// Addr returns the listener address if the transport is in server mode.
// Returns nil if the transport is not currently listening.
// The returned address can be used to determine the actual port when
// listening on port 0 (OS-assigned port).
func (t *tcpTransport) Addr() net.Addr {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.listener != nil {
		return t.listener.Addr()
	}
	return nil
}
