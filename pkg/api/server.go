package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Veraticus/linkpearl/pkg/clipboard"
	linksync "github.com/Veraticus/linkpearl/pkg/sync"
)

// Server implements the local Unix socket API server for linkpearl.
// It provides a simple text-based protocol for clients to interact
// with the running daemon.
type Server struct {
	uptime     time.Time
	clipboard  clipboard.Clipboard
	engine     linksync.Engine
	listener   net.Listener
	ctx        context.Context
	logger     *slog.Logger
	cancel     context.CancelFunc
	socketPath string
	mode       string
	version    string
	nodeID     string
	wg         sync.WaitGroup
	mu         sync.RWMutex
}

// ServerConfig contains configuration for the API server.
type ServerConfig struct {
	Clipboard  clipboard.Clipboard
	Engine     linksync.Engine
	Logger     *slog.Logger
	SocketPath string
	NodeID     string
	Mode       string
	Version    string
}

// NewServer creates a new API server instance.
func NewServer(cfg *ServerConfig) (*Server, error) {
	if cfg.SocketPath == "" {
		return nil, fmt.Errorf("socket path is required")
	}
	if cfg.Clipboard == nil {
		return nil, fmt.Errorf("clipboard is required")
	}
	if cfg.Engine == nil {
		return nil, fmt.Errorf("sync engine is required")
	}

	// Create a discard logger if none provided
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		socketPath: cfg.SocketPath,
		clipboard:  cfg.Clipboard,
		engine:     cfg.Engine,
		nodeID:     cfg.NodeID,
		mode:       cfg.Mode,
		version:    cfg.Version,
		logger:     logger.With("component", "api"),
		uptime:     time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Start begins listening on the Unix domain socket.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create socket directory with proper permissions
	socketDir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove old socket if it exists
	_ = os.Remove(s.socketPath)

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", s.socketPath, err)
	}

	// Set socket permissions (user read/write only)
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	s.listener = listener

	// Start accept loop
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	s.mu.Lock()
	listener := s.listener
	s.mu.Unlock()

	// Cancel context to stop accept loop
	s.cancel()

	// Close listener if it exists
	if listener != nil {
		if err := listener.Close(); err != nil {
			// Ignore "use of closed network connection" errors
			if !isClosedNetworkError(err) {
				return fmt.Errorf("failed to close listener: %w", err)
			}
		}
	}

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	// Wait with timeout
	select {
	case <-done:
		// Clean shutdown
	case <-time.After(5 * time.Second):
		return fmt.Errorf("server shutdown timeout")
	}

	// Remove socket file
	_ = os.Remove(s.socketPath)

	return nil
}

// acceptLoop handles incoming connections.
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				// Server is shutting down
				return
			default:
				if !isClosedNetworkError(err) {
					s.logger.Error("failed to accept connection", "error", err)
					continue
				}
				return
			}
		}

		// Handle connection in a separate goroutine
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(conn)
		}()
	}
}

// handleConnection processes a single client connection.
func (s *Server) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// Set a reasonable timeout for the entire connection
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		s.sendError(conn, "failed to set deadline")
		return
	}

	// Use a buffered reader for the entire connection
	reader := bufio.NewReader(conn)

	// Read command line
	line, err := reader.ReadString('\n')
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to read command: %v", err))
		return
	}

	// Parse request
	req, parseErr := ParseRequest(strings.TrimSpace(line))
	if parseErr != nil {
		s.sendError(conn, parseErr.Error())
		return
	}

	// Handle command
	switch req.Command {
	case CommandCopy:
		s.handleCopy(conn, req, reader)
	case CommandPaste:
		s.handlePaste(conn)
	case CommandStatus:
		s.handleStatus(conn)
	default:
		s.sendError(conn, fmt.Sprintf("Unknown command: %s", req.Command))
	}
}

// handleCopy processes a COPY command.
func (s *Server) handleCopy(conn net.Conn, req *Request, reader *bufio.Reader) {
	// Read content based on size
	content := make([]byte, req.Size)
	// Read from the buffered reader to get the content
	if _, err := io.ReadFull(reader, content); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to read content: %v", err))
		return
	}

	// Validate content
	if err := ValidateContent(content); err != nil {
		s.sendError(conn, err.Error())
		return
	}

	// Send to sync engine for processing and broadcasting
	if err := s.engine.SetClipboard(string(content)); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to set clipboard: %v", err))
		return
	}

	// Send success response
	s.sendOK(conn, nil)
}

// handlePaste processes a PASTE command.
func (s *Server) handlePaste(conn net.Conn) {
	// Read from clipboard
	content, err := s.clipboard.Read()
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to read clipboard: %v", err))
		return
	}

	// Send content
	s.sendOK(conn, content)
}

// handleStatus processes a STATUS command.
func (s *Server) handleStatus(conn net.Conn) {
	s.mu.RLock()
	uptime := s.uptime
	s.mu.RUnlock()

	// Get engine statistics
	stats := s.engine.Stats()

	// Build status response
	status := &StatusResponse{
		NodeID:  s.nodeID,
		Mode:    s.mode,
		Version: s.version,
		Uptime:  uptime,
		Stats: SyncStats{
			MessagesSent:     stats.MessagesSent,
			MessagesReceived: stats.MessagesReceived,
			LocalChanges:     stats.LocalChanges,
			RemoteChanges:    stats.RemoteChanges,
			LastSyncTime:     formatLastSyncTime(stats.LastSyncTime),
		},
	}

	// Get topology information if available
	if topo := s.engine.Topology(); topo != nil {
		status.ListenAddr = topo.ListenAddr()
		status.ConnectedPeers = topo.ConnectedPeers()
		status.JoinAddresses = topo.JoinAddresses()
	}

	// Send status as JSON
	jsonData, err := json.Marshal(status)
	if err != nil {
		s.sendError(conn, fmt.Sprintf("failed to marshal status: %v", err))
		return
	}

	if _, err := fmt.Fprintf(conn, "STATUS %s\n", jsonData); err != nil {
		// Connection error, can't send error response
		return
	}
}

// sendOK sends an OK response with optional data.
func (s *Server) sendOK(conn net.Conn, data any) {
	resp, err := FormatResponse(ResponseOK, data)
	if err != nil {
		s.sendError(conn, err.Error())
		return
	}
	_, _ = conn.Write(resp)
}

// sendError sends an ERROR response.
func (s *Server) sendError(conn net.Conn, msg string) {
	resp, _ := FormatResponse(ResponseError, msg)
	_, _ = conn.Write(resp)
}

// formatLastSyncTime formats the last sync time for JSON output.
func formatLastSyncTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// isClosedNetworkError checks if an error is due to a closed network connection.
func isClosedNetworkError(err error) bool {
	if err == nil {
		return false
	}
	// Check for common closed connection error messages
	errStr := err.Error()
	return errStr == "use of closed network connection" ||
		errStr == "accept unix "+": use of closed file descriptor"
}
