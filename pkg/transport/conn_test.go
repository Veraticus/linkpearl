package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSecureConn(t *testing.T) {
	t.Run("NodeInfo", func(t *testing.T) {
		// Create test TLS connection using net.Pipe
		client, server := net.Pipe()
		defer func() { _ = client.Close() }()
		defer func() { _ = server.Close() }()

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
		// Create test connection pair
		clientConn, serverConn := createTestConnPair(t)
		defer func() { _ = clientConn.Close() }()
		defer func() { _ = serverConn.Close() }()

		// Test message structure
		type TestMessage struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		}

		// Test bidirectional communication
		done := make(chan error, 2)

		// Server goroutine - receives then sends
		go func() {
			var received TestMessage
			if err := serverConn.Receive(&received); err != nil {
				done <- fmt.Errorf("server receive error: %w", err)
				return
			}

			// Verify received message
			if received.ID != "msg1" || received.Content != "Hello from client" {
				done <- fmt.Errorf("server received wrong message: %+v", received)
				return
			}

			// Send response
			response := TestMessage{
				ID:      "msg2",
				Content: "Hello from server",
			}
			if err := serverConn.Send(response); err != nil {
				done <- fmt.Errorf("server send error: %w", err)
				return
			}
			done <- nil
		}()

		// Client operations
		go func() {
			// Send message
			msg := TestMessage{
				ID:      "msg1",
				Content: "Hello from client",
			}
			if err := clientConn.Send(msg); err != nil {
				done <- fmt.Errorf("client send error: %w", err)
				return
			}

			// Receive response
			var response TestMessage
			if err := clientConn.Receive(&response); err != nil {
				done <- fmt.Errorf("client receive error: %w", err)
				return
			}

			// Verify response
			if response.ID != "msg2" || response.Content != "Hello from server" {
				done <- fmt.Errorf("client received wrong response: %+v", response)
				return
			}
			done <- nil
		}()

		// Wait for both goroutines to complete
		for i := 0; i < 2; i++ {
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("Test timeout")
			}
		}
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
		// Create test connection pair
		clientConn, serverConn := createTestConnPair(t)
		defer func() { _ = clientConn.Close() }()
		defer func() { _ = serverConn.Close() }()

		// Test with a message that's within the limit
		type LargeMessage struct {
			Data string `json:"data"`
		}

		// Create a message just under the MaxMessageSize limit
		// Account for JSON overhead (field names, quotes, brackets)
		jsonOverhead := len(`{"data":""}`)
		maxDataSize := MaxMessageSize - jsonOverhead - 1000 // Safety margin
		largeData := make([]byte, maxDataSize)
		for i := range largeData {
			largeData[i] = 'A' + byte(i%26)
		}

		done := make(chan error, 1)

		// Server goroutine - receives the large message
		go func() {
			var received LargeMessage
			if err := serverConn.Receive(&received); err != nil {
				done <- fmt.Errorf("server receive error: %w", err)
				return
			}

			// Verify we received the full message
			if len(received.Data) != maxDataSize {
				done <- fmt.Errorf("received data size mismatch: got %d, want %d",
					len(received.Data), maxDataSize)
				return
			}
			done <- nil
		}()

		// Send the large message
		msg := LargeMessage{
			Data: string(largeData),
		}
		if err := clientConn.Send(msg); err != nil {
			t.Fatalf("Failed to send large message: %v", err)
		}

		// Wait for server to receive
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Test timeout")
		}

		// Now test with a message that exceeds the limit
		// Create a new decoder with size limit to test rejection
		oversizedData := make([]byte, MaxMessageSize+1000)
		for i := range oversizedData {
			oversizedData[i] = 'B' + byte(i%26)
		}

		// Create a pipe for testing message size limit
		testClient, testServer := net.Pipe()
		defer func() { _ = testClient.Close() }()
		defer func() { _ = testServer.Close() }()

		// Create a decoder with size limit
		decoder := json.NewDecoder(io.LimitReader(testServer, MaxMessageSize))

		// Send oversized message
		go func() {
			encoder := json.NewEncoder(testClient)
			oversizedMsg := LargeMessage{
				Data: string(oversizedData),
			}
			_ = encoder.Encode(oversizedMsg)
		}()

		// Try to receive - should fail due to size limit
		var receivedOversized LargeMessage
		err := decoder.Decode(&receivedOversized)
		if err == nil {
			t.Error("Expected error for oversized message, but got none")
		}
		// The error should indicate the message was truncated
		if err != nil && err.Error() != "unexpected EOF" && !strings.Contains(err.Error(), "EOF") {
			t.Errorf("Expected EOF error for oversized message, got: %v", err)
		}
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
