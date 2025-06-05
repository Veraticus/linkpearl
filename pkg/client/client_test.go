package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Veraticus/linkpearl/pkg/api"
	"github.com/Veraticus/linkpearl/pkg/testutil"
)

// mockServer creates a simple Unix socket server for testing.
type mockServer struct {
	listener   net.Listener
	handler    func(conn net.Conn)
	socketPath string
}

func newMockServer(socketPath string, handler func(conn net.Conn)) (*mockServer, error) {
	// Create socket directory
	socketDir := filepath.Dir(socketPath)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return nil, err
	}

	// Remove old socket if exists
	_ = os.Remove(socketPath)

	// Listen on Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	server := &mockServer{
		socketPath: socketPath,
		listener:   listener,
		handler:    handler,
	}

	// Start accepting connections
	go server.acceptLoop()

	return server, nil
}

func (s *mockServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handler(conn)
	}
}

func (s *mockServer) Close() {
	_ = s.listener.Close()
	_ = os.Remove(s.socketPath)
}

func TestDefaultSocketPath(t *testing.T) {
	// Save original env vars
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	origHome := os.Getenv("HOME")
	defer func() {
		_ = os.Setenv("XDG_RUNTIME_DIR", origXDG)
		_ = os.Setenv("HOME", origHome)
	}()

	tests := []struct {
		name string
		xdg  string
		home string
		want string
	}{
		{
			name: "XDG set",
			xdg:  "/run/user/1000",
			home: "/home/user",
			want: "/run/user/1000/linkpearl/linkpearl.sock",
		},
		{
			name: "XDG not set, HOME set",
			xdg:  "",
			home: "/home/user",
			want: "/home/user/.linkpearl/linkpearl.sock",
		},
		{
			name: "Neither set",
			xdg:  "",
			home: "",
			want: "~/.linkpearl/linkpearl.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("XDG_RUNTIME_DIR", tt.xdg)
			_ = os.Setenv("HOME", tt.home)

			got := DefaultSocketPath()
			if got != tt.want {
				t.Errorf("DefaultSocketPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		wantSocket  string
		wantTimeout time.Duration
	}{
		{
			name:        "nil config uses defaults",
			cfg:         nil,
			wantSocket:  DefaultSocketPath(),
			wantTimeout: 5 * time.Second,
		},
		{
			name:        "empty config uses defaults",
			cfg:         &Config{},
			wantSocket:  DefaultSocketPath(),
			wantTimeout: 5 * time.Second,
		},
		{
			name: "custom socket path",
			cfg: &Config{
				SocketPath: "/custom/path.sock",
			},
			wantSocket:  "/custom/path.sock",
			wantTimeout: 5 * time.Second,
		},
		{
			name: "custom timeout",
			cfg: &Config{
				Timeout: 10 * time.Second,
			},
			wantSocket:  DefaultSocketPath(),
			wantTimeout: 10 * time.Second,
		},
		{
			name: "all custom",
			cfg: &Config{
				SocketPath: "/custom/path.sock",
				Timeout:    15 * time.Second,
			},
			wantSocket:  "/custom/path.sock",
			wantTimeout: 15 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(tt.cfg)
			if client.socketPath != tt.wantSocket {
				t.Errorf("socketPath = %v, want %v", client.socketPath, tt.wantSocket)
			}
			if client.timeout != tt.wantTimeout {
				t.Errorf("timeout = %v, want %v", client.timeout, tt.wantTimeout)
			}
		})
	}
}

func TestCopy(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		serverResp  string
		errContains string
		wantErr     bool
	}{
		{
			name:       "successful copy",
			content:    "Hello, World!",
			serverResp: "OK\n",
			wantErr:    false,
		},
		{
			name:       "empty content",
			content:    "",
			serverResp: "OK\n",
			wantErr:    false,
		},
		{
			name:        "server error",
			content:     "test",
			serverResp:  "ERROR clipboard write failed\n",
			wantErr:     true,
			errContains: "copy failed: ERROR clipboard write failed",
		},
		{
			name:        "content too large",
			content:     strings.Repeat("x", 10*1024*1024+1),
			serverResp:  "",
			wantErr:     true,
			errContains: "content validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			socketPath := testutil.SocketPath(t)

			// Create mock server
			server, err := newMockServer(socketPath, func(conn net.Conn) {
				defer func() { _ = conn.Close() }()

				// Use a buffered reader like the real server does
				reader := bufio.NewReader(conn)

				// Read command line
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimSpace(line)

				if strings.HasPrefix(line, "COPY ") {
					// Read content
					var size int
					_, _ = fmt.Sscanf(line, "COPY %d", &size)
					if size > 0 {
						content := make([]byte, size)
						_, _ = io.ReadFull(reader, content)
					}

					// Send response (make sure it ends with newline)
					response := tt.serverResp
					if !strings.HasSuffix(response, "\n") {
						response += "\n"
					}
					_, _ = conn.Write([]byte(response))
				}
			})
			if err != nil {
				t.Fatalf("Failed to create mock server: %v", err)
			}
			defer server.Close()

			// Create client
			client := New(&Config{
				SocketPath: socketPath,
				Timeout:    2 * time.Second,
			})

			// Test copy
			err = client.Copy(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Error("Copy() error = nil, wantErr true")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Copy() error = %v, want error containing %v", err, tt.errContains)
				}
			} else if err != nil {
				t.Errorf("Copy() unexpected error = %v", err)
			}
		})
	}
}

func TestPaste(t *testing.T) {
	tests := []struct {
		name        string
		serverResp  string
		want        string
		errContains string
		wantErr     bool
	}{
		{
			name:       "successful paste",
			serverResp: "OK 13\nHello, World!",
			want:       "Hello, World!",
			wantErr:    false,
		},
		{
			name:       "empty clipboard",
			serverResp: "OK 0\n",
			want:       "",
			wantErr:    false,
		},
		{
			name:        "server error",
			serverResp:  "ERROR clipboard read failed\n",
			want:        "",
			wantErr:     true,
			errContains: "paste failed: ERROR clipboard read failed",
		},
		{
			name:        "invalid response format",
			serverResp:  "INVALID\n",
			want:        "",
			wantErr:     true,
			errContains: "invalid response format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			socketPath := testutil.SocketPath(t)

			// Create mock server
			server, err := newMockServer(socketPath, func(conn net.Conn) {
				defer func() { _ = conn.Close() }()

				scanner := bufio.NewScanner(conn)
				if !scanner.Scan() {
					return
				}

				line := scanner.Text()
				if line == "PASTE" {
					// For PASTE, we need to handle the response correctly:
					// If it's an OK response with content, send the header line first,
					// then the content (without newline)
					if strings.HasPrefix(tt.serverResp, "OK ") {
						lines := strings.SplitN(tt.serverResp, "\n", 2)
						// Send the header line with newline
						_, _ = conn.Write([]byte(lines[0] + "\n"))
						// If there's content, send it without newline
						if len(lines) > 1 {
							_, _ = conn.Write([]byte(lines[1]))
						}
						// Give the client time to read before closing connection
						time.Sleep(10 * time.Millisecond)
					} else {
						// For ERROR responses, send as-is
						_, _ = conn.Write([]byte(tt.serverResp))
					}
				}
			})
			if err != nil {
				t.Fatalf("Failed to create mock server: %v", err)
			}
			defer server.Close()

			// Create client
			client := New(&Config{
				SocketPath: socketPath,
				Timeout:    2 * time.Second,
			})

			// Test paste
			got, err := client.Paste()
			if tt.wantErr {
				if err == nil {
					t.Error("Paste() error = nil, wantErr true")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Paste() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Paste() unexpected error = %v", err)
					return
				}
				if got != tt.want {
					t.Errorf("Paste() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestStatus(t *testing.T) {
	validStatus := &api.StatusResponse{
		NodeID:         "test-node",
		Mode:           "full",
		Version:        "1.0.0",
		Uptime:         time.Now(),
		ListenAddr:     ":8080",
		ConnectedPeers: []string{"peer1", "peer2"},
		Stats: api.SyncStats{
			MessagesSent:     100,
			MessagesReceived: 200,
		},
	}
	validJSON, _ := json.Marshal(validStatus)

	tests := []struct {
		checkResp   func(*api.StatusResponse) error
		name        string
		serverResp  string
		errContains string
		wantErr     bool
	}{
		{
			name:       "successful status",
			serverResp: fmt.Sprintf("STATUS %s\n", validJSON),
			wantErr:    false,
			checkResp: func(resp *api.StatusResponse) error {
				if resp.NodeID != "test-node" {
					return fmt.Errorf("NodeID = %v, want test-node", resp.NodeID)
				}
				if resp.Mode != "full" {
					return fmt.Errorf("Mode = %v, want full", resp.Mode)
				}
				return nil
			},
		},
		{
			name:        "server error",
			serverResp:  "ERROR daemon not ready\n",
			wantErr:     true,
			errContains: "status failed: ERROR daemon not ready",
		},
		{
			name:        "invalid response format",
			serverResp:  "INVALID\n",
			wantErr:     true,
			errContains: "invalid status response",
		},
		{
			name:        "invalid JSON",
			serverResp:  "STATUS {invalid json}\n",
			wantErr:     true,
			errContains: "failed to parse status response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			socketPath := testutil.SocketPath(t)

			// Create mock server
			server, err := newMockServer(socketPath, func(conn net.Conn) {
				defer func() { _ = conn.Close() }()

				scanner := bufio.NewScanner(conn)
				if !scanner.Scan() {
					return
				}

				line := scanner.Text()
				if line == "STATUS" {
					_, _ = conn.Write([]byte(tt.serverResp))
				}
			})
			if err != nil {
				t.Fatalf("Failed to create mock server: %v", err)
			}
			defer server.Close()

			// Create client
			client := New(&Config{
				SocketPath: socketPath,
				Timeout:    2 * time.Second,
			})

			// Test status
			got, err := client.Status()
			if tt.wantErr {
				if err == nil {
					t.Error("Status() error = nil, wantErr true")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Status() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Status() unexpected error = %v", err)
					return
				}
				if tt.checkResp != nil {
					if err := tt.checkResp(got); err != nil {
						t.Errorf("Status() response check failed: %v", err)
					}
				}
			}
		})
	}
}

func TestIsRunning(t *testing.T) {
	socketPath := testutil.SocketPath(t)

	// Test when server is not running
	client := New(&Config{
		SocketPath: socketPath,
		Timeout:    500 * time.Millisecond,
	})

	if client.IsRunning() {
		t.Error("IsRunning() = true when server not running")
	}

	// Start server
	server, err := newMockServer(socketPath, func(conn net.Conn) {
		_ = conn.Close()
	})
	if err != nil {
		t.Fatalf("Failed to create mock server: %v", err)
	}
	defer server.Close()

	// Test when server is running
	if !client.IsRunning() {
		t.Error("IsRunning() = false when server is running")
	}
}

func TestHandleDialError(t *testing.T) {
	// Save original env
	origNeoVim := os.Getenv("LINKPEARL_NEOVIM")
	defer func() { _ = os.Setenv("LINKPEARL_NEOVIM", origNeoVim) }()

	tests := []struct {
		dialErr    error
		name       string
		wantMsg    string
		neoVimMode bool
		wantNil    bool
	}{
		{
			name:       "neovim mode returns nil",
			neoVimMode: true,
			dialErr:    fmt.Errorf("connection refused"),
			wantNil:    true,
		},
		{
			name:       "connection refused",
			neoVimMode: false,
			dialErr:    fmt.Errorf("dial unix /tmp/test.sock: connect: connection refused"),
			wantNil:    false,
			wantMsg:    "linkpearl daemon not running",
		},
		{
			name:       "no such file",
			neoVimMode: false,
			dialErr:    fmt.Errorf("dial unix /tmp/test.sock: connect: no such file or directory"),
			wantNil:    false,
			wantMsg:    "linkpearl daemon not running",
		},
		{
			name:       "other error",
			neoVimMode: false,
			dialErr:    fmt.Errorf("permission denied"),
			wantNil:    false,
			wantMsg:    "failed to connect to daemon: permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.neoVimMode {
				_ = os.Setenv("LINKPEARL_NEOVIM", "1")
			} else {
				_ = os.Setenv("LINKPEARL_NEOVIM", "")
			}

			client := &Client{socketPath: "/tmp/test.sock"}
			err := client.handleDialError(tt.dialErr)

			if tt.wantNil {
				if err != nil {
					t.Errorf("handleDialError() = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Error("handleDialError() = nil, want error")
					return
				}
				if !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("handleDialError() = %v, want containing %v", err, tt.wantMsg)
				}
			}
		})
	}
}

func TestClientTimeout(t *testing.T) {
	socketPath := testutil.SocketPath(t)

	// Create a server that never responds
	server, err := newMockServer(socketPath, func(conn net.Conn) {
		// Never respond, just keep connection open
		time.Sleep(5 * time.Second)
		_ = conn.Close()
	})
	if err != nil {
		t.Fatalf("Failed to create mock server: %v", err)
	}
	defer server.Close()

	// Create client with short timeout
	client := New(&Config{
		SocketPath: socketPath,
		Timeout:    100 * time.Millisecond,
	})

	// All operations should timeout
	start := time.Now()
	err = client.Copy("test")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Copy() should have timed out")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Copy() took too long to timeout: %v", elapsed)
	}
}
