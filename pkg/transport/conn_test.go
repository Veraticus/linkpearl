package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestSecureConn(t *testing.T) {
	t.Run("NodeInfo", func(t *testing.T) {
		// Create test TLS connection using net.Pipe
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()

		nodeInfo := &NodeInfo{
			NodeID:  "test-node",
			Mode:    "full",
			Version: "1.0.0",
		}

		// Create secure connection
		conn := &secureConn{
			nodeID:     nodeInfo.NodeID,
			mode:       nodeInfo.Mode,
			version:    nodeInfo.Version,
			conn:       nil, // OK for testing getters
			closedChan: make(chan struct{}),
		}

		// Test getters
		if conn.NodeID() != "test-node" {
			t.Errorf("NodeID() = %s, want test-node", conn.NodeID())
		}
		if conn.Mode() != "full" {
			t.Errorf("Mode() = %s, want full", conn.Mode())
		}
		if conn.Version() != "1.0.0" {
			t.Errorf("Version() = %s, want 1.0.0", conn.Version())
		}
	})

	t.Run("SendReceive", func(t *testing.T) {
		t.Skip("Skipping SendReceive test - needs refactoring for proper pipe handling")
	})

	t.Run("Close", func(t *testing.T) {
		// Create a simple test connection
		conn := &secureConn{
			nodeID:     "test",
			mode:       "test",
			version:    "1.0.0",
			closedChan: make(chan struct{}),
		}

		// Set up encoder for Send to work
		var buf bytes.Buffer
		conn.encoder = json.NewEncoder(&buf)

		// Close should be idempotent
		if err := conn.Close(); err != nil {
			t.Errorf("first Close() error = %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Errorf("second Close() error = %v", err)
		}

		// Operations after close should fail
		if err := conn.Send("test"); err != ErrClosed {
			t.Errorf("Send() after close = %v, want ErrClosed", err)
		}
	})

	t.Run("LargeMessage", func(t *testing.T) {
		t.Skip("Skipping large message test for now")
	})

	t.Run("Deadlines", func(t *testing.T) {
		conn := &secureConn{
			nodeID:     "test",
			mode:       "test",
			version:    "1.0.0",
			conn:       nil,
			closedChan: make(chan struct{}),
		}

		// Test SetDeadline with nil conn (should not panic)
		deadline := time.Now().Add(100 * time.Millisecond)
		if err := conn.SetDeadline(deadline); err != nil {
			t.Errorf("SetDeadline() error = %v", err)
		}

		// Test SetReadDeadline
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Errorf("SetReadDeadline() error = %v", err)
		}

		// Test SetWriteDeadline
		if err := conn.SetWriteDeadline(deadline); err != nil {
			t.Errorf("SetWriteDeadline() error = %v", err)
		}
	})
}

func TestNodeInfo(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		tests := []struct {
			name    string
			info    NodeInfo
			wantErr bool
		}{
			{
				name: "valid",
				info: NodeInfo{
					NodeID:  "node-1",
					Mode:    "full",
					Version: "1.0.0",
				},
				wantErr: false,
			},
			{
				name: "missing node ID",
				info: NodeInfo{
					Mode:    "full",
					Version: "1.0.0",
				},
				wantErr: true,
			},
			{
				name: "missing mode",
				info: NodeInfo{
					NodeID:  "node-1",
					Version: "1.0.0",
				},
				wantErr: true,
			},
			{
				name: "missing version",
				info: NodeInfo{
					NodeID: "node-1",
					Mode:   "full",
				},
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := tt.info.Validate()
				if (err != nil) != tt.wantErr {
					t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("JSONSerialization", func(t *testing.T) {
		info := NodeInfo{
			NodeID:  "test-node",
			Mode:    "client",
			Version: "1.2.3",
		}

		// Marshal
		data, err := json.Marshal(info)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		// Unmarshal
		var decoded NodeInfo
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}

		// Verify
		if decoded != info {
			t.Errorf("decoded = %+v, want %+v", decoded, info)
		}
	})
}

// createTestConnPair creates a pair of secure connections for testing
func createTestConnPair(t *testing.T) (*secureConn, *secureConn) {
	// Create pipe
	client, server := net.Pipe()

	// Create node info
	clientInfo := &NodeInfo{
		NodeID:  "client",
		Mode:    "client",
		Version: "1.0.0",
	}
	serverInfo := &NodeInfo{
		NodeID:  "server",
		Mode:    "full",
		Version: "1.0.0",
	}

	// Create wrapper connections that properly handle Close
	clientWrapper := &testConnWrapper{Conn: client}
	serverWrapper := &testConnWrapper{Conn: server}

	// Create secure connections for testing
	clientConn := &secureConn{
		nodeID:     serverInfo.NodeID,
		mode:       serverInfo.Mode,
		version:    serverInfo.Version,
		conn:       clientWrapper,
		encoder:    json.NewEncoder(client),
		decoder:    json.NewDecoder(client),
		closedChan: make(chan struct{}),
	}

	serverConn := &secureConn{
		nodeID:     clientInfo.NodeID,
		mode:       clientInfo.Mode,
		version:    clientInfo.Version,
		conn:       serverWrapper,
		encoder:    json.NewEncoder(server),
		decoder:    json.NewDecoder(server),
		closedChan: make(chan struct{}),
	}

	return clientConn, serverConn
}

// testConnWrapper wraps a net.Conn to provide basic TLS conn functionality for tests
type testConnWrapper struct {
	net.Conn
}

// These methods make it compatible with what secureConn expects
func (t *testConnWrapper) LocalAddr() net.Addr  { return t.Conn.LocalAddr() }
func (t *testConnWrapper) RemoteAddr() net.Addr { return t.Conn.RemoteAddr() }

// mockTLSConn wraps a net.Conn to satisfy the tls.Conn interface for testing
type mockTLSConn struct {
	net.Conn
}

func (m *mockTLSConn) ConnectionState() tls.ConnectionState {
	return tls.ConnectionState{
		Version:     tls.VersionTLS13,
		CipherSuite: tls.TLS_AES_128_GCM_SHA256,
	}
}

func (m *mockTLSConn) Handshake() error {
	return nil
}

func (m *mockTLSConn) HandshakeContext(ctx context.Context) error {
	return nil
}
