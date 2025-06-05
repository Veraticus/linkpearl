// Package client provides a client library for interacting with the linkpearl daemon
// via its Unix socket API.
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Veraticus/linkpearl/pkg/api"
)

// Client provides methods to interact with a running linkpearl daemon.
type Client struct {
	socketPath string
	timeout    time.Duration
}

// Config contains configuration for the client.
type Config struct {
	// SocketPath is the path to the Unix domain socket.
	// If empty, uses the default socket path.
	SocketPath string

	// Timeout for operations. Default is 5 seconds.
	Timeout time.Duration
}

// DefaultSocketPath returns the default socket path based on XDG standards.
func DefaultSocketPath() string {
	// Check XDG_RUNTIME_DIR first
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return xdg + "/linkpearl/linkpearl.sock"
	}
	// Fall back to home directory
	home := os.Getenv("HOME")
	if home == "" {
		home = "~"
	}
	return home + "/.linkpearl/linkpearl.sock"
}

// New creates a new client with the given configuration.
func New(cfg *Config) *Client {
	if cfg == nil {
		cfg = &Config{}
	}

	socketPath := cfg.SocketPath
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &Client{
		socketPath: socketPath,
		timeout:    timeout,
	}
}

// Copy sends text to the daemon's clipboard.
func (c *Client) Copy(content string) error {
	// Validate content size
	if err := api.ValidateContent([]byte(content)); err != nil {
		return fmt.Errorf("content validation failed: %w", err)
	}

	conn, err := c.dial()
	if err != nil {
		return c.handleDialError(err)
	}
	defer func() { _ = conn.Close() }()

	// Send COPY command with size
	if _, writeErr := fmt.Fprintf(conn, "COPY %d\n%s", len(content), content); writeErr != nil {
		return fmt.Errorf("failed to send copy command: %w", writeErr)
	}

	// Read response
	response, err := c.readResponse(conn)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(response, "OK") {
		return fmt.Errorf("copy failed: %s", response)
	}

	return nil
}

// Paste retrieves the current clipboard content from the daemon.
func (c *Client) Paste() (string, error) {
	conn, err := c.dial()
	if err != nil {
		return "", c.handleDialError(err)
	}
	defer func() { _ = conn.Close() }()

	// Send PASTE command
	if _, writeErr := fmt.Fprintln(conn, "PASTE"); writeErr != nil {
		return "", fmt.Errorf("failed to send paste command: %w", writeErr)
	}

	// Use a buffered reader to avoid buffering issues
	reader := bufio.NewReader(conn)

	// Read response line
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	response = strings.TrimSpace(response)

	if strings.HasPrefix(response, "ERROR") {
		return "", fmt.Errorf("paste failed: %s", response)
	}

	// Parse OK response with size
	var size int
	if _, err := fmt.Sscanf(response, "OK %d", &size); err != nil {
		return "", fmt.Errorf("invalid response format: %s", response)
	}

	// Read content based on size
	if size == 0 {
		return "", nil
	}

	content := make([]byte, size)
	if _, err := io.ReadFull(reader, content); err != nil {
		return "", fmt.Errorf("failed to read content: %w", err)
	}

	return string(content), nil
}

// Status retrieves the daemon's current status.
func (c *Client) Status() (*api.StatusResponse, error) {
	conn, err := c.dial()
	if err != nil {
		return nil, c.handleDialError(err)
	}
	defer func() { _ = conn.Close() }()

	// Send STATUS command
	if _, writeErr := fmt.Fprintln(conn, "STATUS"); writeErr != nil {
		return nil, fmt.Errorf("failed to send status command: %w", writeErr)
	}

	// Read response
	response, err := c.readResponse(conn)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(response, "ERROR") {
		return nil, fmt.Errorf("status failed: %s", response)
	}

	// Parse STATUS response
	if !strings.HasPrefix(response, "STATUS ") {
		return nil, fmt.Errorf("invalid status response: %s", response)
	}

	jsonData := strings.TrimPrefix(response, "STATUS ")
	var status api.StatusResponse
	if err := json.Unmarshal([]byte(jsonData), &status); err != nil {
		return nil, fmt.Errorf("failed to parse status response: %w", err)
	}

	return &status, nil
}

// IsRunning checks if the daemon is running and responsive.
func (c *Client) IsRunning() bool {
	conn, err := c.dial()
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// dial establishes a connection to the daemon's socket.
func (c *Client) dial() (net.Conn, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, err
	}

	// Set deadline for all operations
	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to set connection deadline: %w", err)
	}

	return conn, nil
}

// readResponse reads a single line response from the connection.
func (c *Client) readResponse(conn net.Conn) (string, error) {
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return "", fmt.Errorf("no response from daemon")
		}
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	return strings.TrimSpace(response), nil
}

// handleDialError provides appropriate error messages for connection failures.
func (c *Client) handleDialError(err error) error {
	// Check if running in NeoVim mode
	if os.Getenv("LINKPEARL_NEOVIM") == "1" {
		// Fail silently for NeoVim integration
		return nil
	}

	// Check for common connection refused errors
	if strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "no such file") {
		return fmt.Errorf("linkpearl daemon not running (socket: %s)", c.socketPath)
	}

	return fmt.Errorf("failed to connect to daemon: %w", err)
}
