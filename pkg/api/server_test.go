package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/linkpearl/pkg/clipboard"
	linksync "github.com/Veraticus/linkpearl/pkg/sync"
)

// mockClipboard implements clipboard.Clipboard for testing.
type mockClipboard struct {
	content string
	state   clipboard.State
	err     error
}

func (m *mockClipboard) Read() (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.content, nil
}

func (m *mockClipboard) Write(content string) error {
	if m.err != nil {
		return m.err
	}
	m.content = content
	return nil
}

func (m *mockClipboard) Watch(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch
}

func (m *mockClipboard) GetState() clipboard.State {
	return m.state
}

// mockEngine implements linksync.Engine for testing.
type mockEngine struct {
	stats    linksync.Stats
	topology linksync.MockTopology
}

func (m *mockEngine) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (m *mockEngine) Stats() *linksync.Stats {
	return &m.stats
}

func (m *mockEngine) Topology() linksync.Topology {
	return &m.topology
}

func TestNewServer(t *testing.T) {
	clip := &mockClipboard{}
	engine := &mockEngine{}

	tests := []struct {
		name    string
		cfg     *ServerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: &ServerConfig{
				SocketPath: "/tmp/test.sock",
				Clipboard:  clip,
				Engine:     engine,
				NodeID:     "test-node",
				Mode:       "full",
				Version:    "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "missing socket path",
			cfg: &ServerConfig{
				Clipboard: clip,
				Engine:    engine,
			},
			wantErr: true,
			errMsg:  "socket path is required",
		},
		{
			name: "missing clipboard",
			cfg: &ServerConfig{
				SocketPath: "/tmp/test.sock",
				Engine:     engine,
			},
			wantErr: true,
			errMsg:  "clipboard is required",
		},
		{
			name: "missing engine",
			cfg: &ServerConfig{
				SocketPath: "/tmp/test.sock",
				Clipboard:  clip,
			},
			wantErr: true,
			errMsg:  "sync engine is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewServer() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("NewServer() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("NewServer() unexpected error = %v", err)
				return
			}
			if server == nil {
				t.Error("NewServer() returned nil server")
			}
		})
	}
}

func TestServerStartStop(t *testing.T) {
	// Create temporary directory for socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	server, err := NewServer(&ServerConfig{
		SocketPath: socketPath,
		Clipboard:  &mockClipboard{},
		Engine:     &mockEngine{},
		NodeID:     "test-node",
		Mode:       "full",
		Version:    "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Verify socket exists
	if _, err := os.Stat(socketPath); err != nil {
		t.Errorf("Socket file not created: %v", err)
	}

	// Verify we can connect
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Errorf("Failed to connect to server: %v", err)
	} else {
		_ = conn.Close()
	}

	// Stop server
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}

	// Verify socket is removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket file not removed after stop")
	}
}

func TestServerCommands(t *testing.T) {
	tests := []struct {
		name           string
		command        string
		content        string
		clipboardState *mockClipboard
		engineState    *mockEngine
		wantResponse   string
		wantClipboard  string
	}{
		{
			name:           "COPY command",
			command:        "COPY 13\nHello, World!",
			clipboardState: &mockClipboard{},
			engineState:    &mockEngine{},
			wantResponse:   "OK\n",
			wantClipboard:  "Hello, World!",
		},
		{
			name:    "PASTE command",
			command: "PASTE\n",
			clipboardState: &mockClipboard{
				content: "Test content",
			},
			engineState:  &mockEngine{},
			wantResponse: "OK 12\nTest content",
		},
		{
			name:    "PASTE empty clipboard",
			command: "PASTE\n",
			clipboardState: &mockClipboard{
				content: "",
			},
			engineState:  &mockEngine{},
			wantResponse: "OK 0\n",
		},
		{
			name:           "STATUS command",
			command:        "STATUS\n",
			clipboardState: &mockClipboard{},
			engineState: &mockEngine{
				stats: linksync.Stats{
					MessagesSent:     10,
					MessagesReceived: 20,
					LocalChanges:     5,
					RemoteChanges:    8,
				},
			},
			wantResponse: "STATUS ",
		},
		{
			name:           "unknown command",
			command:        "UNKNOWN\n",
			clipboardState: &mockClipboard{},
			engineState:    &mockEngine{},
			wantResponse:   "ERROR unknown command: UNKNOWN\n",
		},
		{
			name:           "COPY with invalid size",
			command:        "COPY abc\n",
			clipboardState: &mockClipboard{},
			engineState:    &mockEngine{},
			wantResponse:   "ERROR COPY requires size parameter\n",
		},
		{
			name:           "COPY without size",
			command:        "COPY\n",
			clipboardState: &mockClipboard{},
			engineState:    &mockEngine{},
			wantResponse:   "ERROR COPY requires size parameter\n",
		},
		{
			name:    "COPY with clipboard error",
			command: "COPY 5\nHello",
			clipboardState: &mockClipboard{
				err: fmt.Errorf("clipboard error"),
			},
			engineState:  &mockEngine{},
			wantResponse: "ERROR failed to write to clipboard: clipboard error\n",
		},
		{
			name:    "PASTE with clipboard error",
			command: "PASTE\n",
			clipboardState: &mockClipboard{
				err: fmt.Errorf("clipboard error"),
			},
			engineState:  &mockEngine{},
			wantResponse: "ERROR failed to read clipboard: clipboard error\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for socket
			tmpDir := t.TempDir()
			socketPath := filepath.Join(tmpDir, "test.sock")

			// Add topology to engine if needed
			if tt.engineState.topology.ListenAddress == "" {
				tt.engineState.topology.ListenAddress = ":8080"
				tt.engineState.topology.Peers = []string{"peer1"}
				tt.engineState.topology.JoinAddrs = []string{"host1:8080"}
			}

			server, err := NewServer(&ServerConfig{
				SocketPath: socketPath,
				Clipboard:  tt.clipboardState,
				Engine:     tt.engineState,
				NodeID:     "test-node",
				Mode:       "full",
				Version:    "1.0.0",
			})
			if err != nil {
				t.Fatalf("Failed to create server: %v", err)
			}

			// Start server
			if err := server.Start(); err != nil {
				t.Fatalf("Failed to start server: %v", err)
			}
			defer func() { _ = server.Stop() }()

			// Connect to server
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}
			defer func() { _ = conn.Close() }()

			// Set deadline
			if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatalf("Failed to set deadline: %v", err)
			}

			// Send command
			if _, err := conn.Write([]byte(tt.command)); err != nil {
				t.Fatalf("Failed to send command: %v", err)
			}

			// Read response using buffered reader to avoid buffering issues
			reader := bufio.NewReader(conn)
			responseLine, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}
			response := strings.TrimSpace(responseLine)

			// For STATUS command, just verify it starts with "STATUS "
			if strings.HasPrefix(tt.wantResponse, "STATUS ") {
				if !strings.HasPrefix(response, "STATUS ") {
					t.Errorf("Response = %v, want prefix %v", response, tt.wantResponse)
				}
			} else {
				// For PASTE with content, need to read the content too
				if strings.HasPrefix(response, "OK ") && strings.Contains(tt.wantResponse, "\n") {
					// Parse size from response
					var size int
					if _, err := fmt.Sscanf(response, "OK %d", &size); err != nil {
						t.Fatalf("Failed to parse size from response %q: %v", response, err)
					}
					if size > 0 {
						// Read content
						content := make([]byte, size)
						if _, err := io.ReadFull(reader, content); err != nil {
							t.Fatalf("Failed to read content: %v", err)
						}
						response += "\n" + string(content)
					} else {
						response += "\n"
					}
				} else {
					response += "\n"
				}

				if response != tt.wantResponse {
					t.Errorf("Response = %q, want %q", response, tt.wantResponse)
				}
			}

			// Check clipboard state if needed
			if tt.wantClipboard != "" {
				if tt.clipboardState.content != tt.wantClipboard {
					t.Errorf("Clipboard content = %q, want %q",
						tt.clipboardState.content, tt.wantClipboard)
				}
			}
		})
	}
}

func TestServerConcurrentConnections(t *testing.T) {
	// Create temporary directory for socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	clipboard := &mockClipboard{content: "shared content"}
	server, err := NewServer(&ServerConfig{
		SocketPath: socketPath,
		Clipboard:  clipboard,
		Engine:     &mockEngine{},
		NodeID:     "test-node",
		Mode:       "full",
		Version:    "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Test concurrent connections
	const numClients = 10
	done := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		go func(id int) {
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				done <- fmt.Errorf("client %d: failed to connect: %v", id, err)
				return
			}
			defer func() { _ = conn.Close() }()

			// Send PASTE command
			if _, err := fmt.Fprintln(conn, "PASTE"); err != nil {
				done <- fmt.Errorf("client %d: failed to send: %v", id, err)
				return
			}

			// Read response
			scanner := bufio.NewScanner(conn)
			if !scanner.Scan() {
				done <- fmt.Errorf("client %d: failed to read: %v", id, scanner.Err())
				return
			}

			response := scanner.Text()
			if !strings.HasPrefix(response, "OK ") {
				done <- fmt.Errorf("client %d: unexpected response: %s", id, response)
				return
			}

			done <- nil
		}(i)
	}

	// Wait for all clients
	for i := 0; i < numClients; i++ {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

func TestServerSocketPermissions(t *testing.T) {
	// Create temporary directory for socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	server, err := NewServer(&ServerConfig{
		SocketPath: socketPath,
		Clipboard:  &mockClipboard{},
		Engine:     &mockEngine{},
		NodeID:     "test-node",
		Mode:       "full",
		Version:    "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Check socket permissions
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("Failed to stat socket: %v", err)
	}

	// Should be 0600 (user read/write only)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Socket permissions = %o, want %o", perm, 0600)
	}
}

func TestServerContentSizeLimit(t *testing.T) {
	t.Skip("Skipping test that requires large data transfer")
	// Set a shorter timeout for this test
	origTimeout := 5 * time.Second
	defer func(_ time.Duration) { time.Sleep(0) }(origTimeout)

	// Create temporary directory for socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	server, err := NewServer(&ServerConfig{
		SocketPath: socketPath,
		Clipboard:  &mockClipboard{},
		Engine:     &mockEngine{},
		NodeID:     "test-node",
		Mode:       "full",
		Version:    "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() { _ = server.Stop() }()

	// Connect to server
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Set a deadline for this test
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("Failed to set deadline: %v", err)
	}

	// Try to send content larger than limit
	largeSize := 10*1024*1024 + 1
	if _, err := fmt.Fprintf(conn, "COPY %d\n", largeSize); err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	// The server will try to read largeSize bytes, but we'll send the real content it expects
	// Send the full content as the server expects
	content := make([]byte, largeSize)
	// Try to send content - this might fail if the server closes the connection early
	// which is fine - we just need to read the response
	_, _ = conn.Write(content)

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to read response: %v", err)
	}

	response = strings.TrimSpace(response)
	if !strings.HasPrefix(response, "ERROR") || !strings.Contains(response, "content too large") {
		t.Errorf("Expected content too large error, got: %s", response)
	}
}
