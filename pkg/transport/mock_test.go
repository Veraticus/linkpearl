package transport

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// mockTransport implements Transport for testing
type mockTransport struct {
	nodeID   string
	mode     string
	secret   string
	listener net.Listener
	conns    map[string]Conn
	mu       sync.RWMutex
	closed   bool
}

func newMockTransport(nodeID, mode, secret string) *mockTransport {
	return &mockTransport{
		nodeID: nodeID,
		mode:   mode,
		secret: secret,
		conns:  make(map[string]Conn),
	}
}

func (m *mockTransport) Listen(addr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrClosed
	}

	if m.listener != nil {
		return errAlreadyListening
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	m.listener = listener
	return nil
}

func (m *mockTransport) Accept() (Conn, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrClosed
	}
	listener := m.listener
	m.mu.RUnlock()

	if listener == nil {
		return nil, errNotListening
	}

	// For mock, just create a fake connection
	conn := &mockConn{
		nodeID:  "mock-client",
		mode:    "client",
		version: "1.0.0",
	}

	return conn, nil
}

func (m *mockTransport) Connect(ctx context.Context, addr string) (Conn, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrClosed
	}
	m.mu.RUnlock()

	// For mock, just create a fake connection
	conn := &mockConn{
		nodeID:  "mock-server",
		mode:    "full",
		version: "1.0.0",
	}

	return conn, nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	if m.listener != nil {
		m.listener.Close()
		m.listener = nil
	}

	for _, conn := range m.conns {
		conn.Close()
	}
	m.conns = make(map[string]Conn)

	return nil
}

func (m *mockTransport) Addr() net.Addr {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.listener != nil {
		return m.listener.Addr()
	}
	return nil
}

// mockConn implements Conn for testing
type mockConn struct {
	nodeID   string
	mode     string
	version  string
	messages chan interface{}
	closed   bool
	mu       sync.Mutex
}

func (m *mockConn) NodeID() string  { return m.nodeID }
func (m *mockConn) Mode() string    { return m.mode }
func (m *mockConn) Version() string { return m.version }

func (m *mockConn) Send(msg interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrClosed
	}

	if m.messages == nil {
		m.messages = make(chan interface{}, 10)
	}

	select {
	case m.messages <- msg:
		return nil
	default:
		return errBufferFull
	}
}

func (m *mockConn) Receive(msg interface{}) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	if m.messages == nil {
		m.messages = make(chan interface{}, 10)
	}
	m.mu.Unlock()

	select {
	case received := <-m.messages:
		// Simple type assertion for testing
		switch v := msg.(type) {
		case *string:
			if s, ok := received.(string); ok {
				*v = s
			}
		case *interface{}:
			*v = received
		}
		return nil
	default:
		return errNoData
	}
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	if m.messages != nil {
		close(m.messages)
	}

	return nil
}

func (m *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// Test errors
var (
	errAlreadyListening = fmt.Errorf("already listening")
	errNotListening     = fmt.Errorf("not listening")
	errBufferFull       = fmt.Errorf("buffer full")
	errNoData           = fmt.Errorf("no data available")
)
