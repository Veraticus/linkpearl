package transport

import (
	"testing"
)

// TestSecureConnSimple tests basic functionality without complex I/O
func TestSecureConnSimple(t *testing.T) {
	t.Run("BasicProperties", func(t *testing.T) {
		conn := &secureConn{
			nodeID:     "test-node",
			mode:       "full",
			version:    "1.0.0",
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

		// Test addresses with nil conn
		if addr := conn.LocalAddr(); addr != nil {
			t.Errorf("LocalAddr() = %v, want nil", addr)
		}
		if addr := conn.RemoteAddr(); addr != nil {
			t.Errorf("RemoteAddr() = %v, want nil", addr)
		}
	})

	t.Run("CloseIdempotency", func(t *testing.T) {
		conn := &secureConn{
			nodeID:     "test",
			mode:       "test",
			version:    "1.0.0",
			closedChan: make(chan struct{}),
		}

		// First close
		if err := conn.Close(); err != nil {
			t.Errorf("first Close() error = %v", err)
		}

		// Verify closed
		if !conn.closed {
			t.Error("conn should be marked as closed")
		}

		// Second close should be safe
		if err := conn.Close(); err != nil {
			t.Errorf("second Close() error = %v", err)
		}
	})
}
