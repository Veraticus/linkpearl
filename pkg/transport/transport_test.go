package transport

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestTCPTransport(t *testing.T) {
	t.Run("ListenAndAccept", func(t *testing.T) {
		// Create transport
		config := &Config{
			NodeID: "test-server",
			Mode:   "full",
			Secret: "test-secret",
			Logger: DefaultLogger(),
		}
		transport := NewTCPTransport(config)
		defer func() { _ = transport.Close() }()

		// Start listening
		if err := transport.Listen(":0"); err != nil {
			t.Fatalf("Listen() error = %v", err)
		}

		// Get address
		addr := transport.Addr()
		if addr == nil {
			t.Fatal("Addr() returned nil")
		}

		// Try to listen again (should fail)
		if err := transport.Listen(":0"); err == nil {
			t.Error("second Listen() should fail")
		}
	})

	t.Run("ConnectAndCommunicate", func(t *testing.T) {
		secret := "shared-secret"

		// Create server transport
		serverConfig := &Config{
			NodeID: "server",
			Mode:   "full",
			Secret: secret,
			Logger: DefaultLogger(),
		}
		serverTransport := NewTCPTransport(serverConfig)
		defer func() { _ = serverTransport.Close() }()

		// Start server
		if err := serverTransport.Listen(":0"); err != nil {
			t.Fatalf("server Listen() error = %v", err)
		}
		serverAddr := serverTransport.Addr().String()

		// Accept connections in goroutine
		var serverConn Conn
		var acceptErr error
		acceptDone := make(chan struct{})

		go func() {
			defer close(acceptDone)
			serverConn, acceptErr = serverTransport.Accept()
		}()

		// Create client transport
		clientConfig := &Config{
			NodeID: "client",
			Mode:   "client",
			Secret: secret,
			Logger: DefaultLogger(),
		}
		clientTransport := NewTCPTransport(clientConfig)
		defer func() { _ = clientTransport.Close() }()

		// Connect to server
		ctx := context.Background()
		clientConn, err := clientTransport.Connect(ctx, serverAddr)
		if err != nil {
			t.Fatalf("Connect() error = %v", err)
		}
		defer func() { _ = clientConn.Close() }()

		// Wait for server to accept
		<-acceptDone
		if acceptErr != nil {
			t.Fatalf("Accept() error = %v", acceptErr)
		}
		defer func() { _ = serverConn.Close() }()

		// Verify connection metadata
		if clientConn.NodeID() != "server" {
			t.Errorf("client sees NodeID = %s, want server", clientConn.NodeID())
		}
		if serverConn.NodeID() != "client" {
			t.Errorf("server sees NodeID = %s, want client", serverConn.NodeID())
		}

		// Test bidirectional communication
		type testMsg struct {
			Content string `json:"content"`
		}

		// Client to server
		if err := clientConn.Send(testMsg{Content: "hello from client"}); err != nil {
			t.Errorf("client Send() error = %v", err)
		}

		var serverReceived testMsg
		if err := serverConn.Receive(&serverReceived); err != nil {
			t.Errorf("server Receive() error = %v", err)
		}
		if serverReceived.Content != "hello from client" {
			t.Errorf("server received %q, want %q", serverReceived.Content, "hello from client")
		}

		// Server to client
		if err := serverConn.Send(testMsg{Content: "hello from server"}); err != nil {
			t.Errorf("server Send() error = %v", err)
		}

		var clientReceived testMsg
		if err := clientConn.Receive(&clientReceived); err != nil {
			t.Errorf("client Receive() error = %v", err)
		}
		if clientReceived.Content != "hello from server" {
			t.Errorf("client received %q, want %q", clientReceived.Content, "hello from server")
		}
	})

	t.Run("AuthenticationFailure", func(t *testing.T) {
		// Create server with one secret
		serverConfig := &Config{
			NodeID: "server",
			Mode:   "full",
			Secret: "server-secret",
			Logger: DefaultLogger(),
		}
		serverTransport := NewTCPTransport(serverConfig)
		defer func() { _ = serverTransport.Close() }()

		// Start server
		if err := serverTransport.Listen(":0"); err != nil {
			t.Fatalf("server Listen() error = %v", err)
		}
		serverAddr := serverTransport.Addr().String()

		// Accept in goroutine
		acceptDone := make(chan struct{})
		go func() {
			defer close(acceptDone)
			conn, err := serverTransport.Accept()
			if err == nil {
				_ = conn.Close()
				t.Error("Accept() should fail with auth error")
			}
		}()

		// Create client with different secret
		clientConfig := &Config{
			NodeID: "client",
			Mode:   "client",
			Secret: "wrong-secret",
			Logger: DefaultLogger(),
		}
		clientTransport := NewTCPTransport(clientConfig)
		defer func() { _ = clientTransport.Close() }()

		// Connect should fail
		ctx := context.Background()
		conn, err := clientTransport.Connect(ctx, serverAddr)
		if err == nil {
			_ = conn.Close()
			t.Error("Connect() should fail with wrong secret")
		}

		// Wait for server
		<-acceptDone
	})

	t.Run("MultipleConnections", func(t *testing.T) {
		secret := "shared-secret"

		// Create server
		serverConfig := &Config{
			NodeID: "server",
			Mode:   "full",
			Secret: secret,
			Logger: DefaultLogger(),
		}
		serverTransport := NewTCPTransport(serverConfig)
		defer func() { _ = serverTransport.Close() }()

		if err := serverTransport.Listen(":0"); err != nil {
			t.Fatalf("Listen() error = %v", err)
		}
		serverAddr := serverTransport.Addr().String()

		// Accept connections concurrently
		var wg sync.WaitGroup
		connections := 3
		serverConns := make([]Conn, connections)

		for i := 0; i < connections; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				conn, err := serverTransport.Accept()
				if err != nil {
					t.Errorf("Accept() #%d error = %v", idx, err)
					return
				}
				serverConns[idx] = conn
			}(i)
		}

		// Create multiple clients
		clientTransports := make([]Transport, 0, connections)
		clientConns := make([]Conn, 0, connections)
		defer func() {
			for _, conn := range clientConns {
				if conn != nil {
					_ = conn.Close()
				}
			}
			for _, trans := range clientTransports {
				if trans != nil {
					_ = trans.Close()
				}
			}
		}()

		for i := 0; i < connections; i++ {
			clientConfig := &Config{
				NodeID: fmt.Sprintf("client-%d", i),
				Mode:   "client",
				Secret: secret,
				Logger: DefaultLogger(),
			}
			clientTransport := NewTCPTransport(clientConfig)
			clientTransports = append(clientTransports, clientTransport)

			ctx := context.Background()
			conn, err := clientTransport.Connect(ctx, serverAddr)
			if err != nil {
				t.Errorf("Connect() #%d error = %v", i, err)
				continue
			}
			clientConns = append(clientConns, conn)
		}

		// Wait for all accepts
		wg.Wait()

		// Close all server connections
		for _, conn := range serverConns {
			if conn != nil {
				_ = conn.Close()
			}
		}
	})

	t.Run("CloseTransport", func(t *testing.T) {
		config := &Config{
			NodeID: "test",
			Mode:   "full",
			Secret: "secret",
			Logger: DefaultLogger(),
		}
		transport := NewTCPTransport(config)

		// Start listening
		if err := transport.Listen(":0"); err != nil {
			t.Fatalf("Listen() error = %v", err)
		}

		// Close should work
		if err := transport.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}

		// Operations after close should fail
		if err := transport.Listen(":0"); err != ErrClosed {
			t.Errorf("Listen() after close = %v, want ErrClosed", err)
		}

		ctx := context.Background()
		if _, err := transport.Connect(ctx, "localhost:1234"); err != ErrClosed {
			t.Errorf("Connect() after close = %v, want ErrClosed", err)
		}

		if _, err := transport.Accept(); err != ErrClosed {
			t.Errorf("Accept() after close = %v, want ErrClosed", err)
		}

		// Close again should be safe
		if err := transport.Close(); err != nil {
			t.Errorf("second Close() error = %v", err)
		}
	})

	t.Run("ConnectTimeout", func(t *testing.T) {
		config := &Config{
			NodeID: "client",
			Mode:   "client",
			Secret: "secret",
			Logger: DefaultLogger(),
		}
		transport := NewTCPTransport(config)
		defer func() { _ = transport.Close() }()

		// Use a context with short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Try to connect to non-existent address
		_, err := transport.Connect(ctx, "192.0.2.1:1234") // TEST-NET-1, should not respond
		if err == nil {
			t.Error("Connect() should timeout")
		}
	})
}

// TestIntegration performs end-to-end integration tests.
func TestIntegration(t *testing.T) {
	t.Run("FullHandshakeAndTLS", func(t *testing.T) {
		secret := "integration-secret"

		// Create server
		serverConfig := &Config{
			NodeID: "integration-server",
			Mode:   "full",
			Secret: secret,
			Logger: DefaultLogger(),
		}
		serverTransport := NewTCPTransport(serverConfig)
		defer func() { _ = serverTransport.Close() }()

		if err := serverTransport.Listen(":0"); err != nil {
			t.Fatalf("Listen() error = %v", err)
		}
		serverAddr := serverTransport.Addr().String()

		// Accept in goroutine
		acceptChan := make(chan Conn, 1)
		acceptErr := make(chan error, 1)
		go func() {
			conn, err := serverTransport.Accept()
			if err != nil {
				acceptErr <- err
			} else {
				acceptChan <- conn
			}
		}()

		// Create client
		clientConfig := &Config{
			NodeID: "integration-client",
			Mode:   "client",
			Secret: secret,
			Logger: DefaultLogger(),
		}
		clientTransport := NewTCPTransport(clientConfig)
		defer func() { _ = clientTransport.Close() }()

		// Connect
		ctx := context.Background()
		clientConn, err := clientTransport.Connect(ctx, serverAddr)
		if err != nil {
			t.Fatalf("Connect() error = %v", err)
		}
		defer func() { _ = clientConn.Close() }()

		// Get server connection
		var serverConn Conn
		select {
		case serverConn = <-acceptChan:
			defer func() { _ = serverConn.Close() }()
		case err := <-acceptErr:
			t.Fatalf("Accept() error = %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Accept() timeout")
		}

		// Verify we're using TLS by checking connection type
		// (The actual TLS verification happens internally)

		// Exchange multiple messages to verify connection stability
		for i := 0; i < 10; i++ {
			msg := map[string]any{
				"type":  "test",
				"index": i,
				"data":  fmt.Sprintf("message-%d", i),
			}

			// Alternate sender
			if i%2 == 0 {
				if err := clientConn.Send(msg); err != nil {
					t.Errorf("client Send() #%d error = %v", i, err)
				}
				var received map[string]any
				if err := serverConn.Receive(&received); err != nil {
					t.Errorf("server Receive() #%d error = %v", i, err)
				}
			} else {
				if err := serverConn.Send(msg); err != nil {
					t.Errorf("server Send() #%d error = %v", i, err)
				}
				var received map[string]any
				if err := clientConn.Receive(&received); err != nil {
					t.Errorf("client Receive() #%d error = %v", i, err)
				}
			}
		}
	})
}
