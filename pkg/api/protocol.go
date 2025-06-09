// Package api provides the local Unix socket API for client-server communication.
// This enables headless server integration and allows multiple clients to interact
// with a running linkpearl daemon.
package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// Command represents the type of command sent by the client.
type Command string

// Command constants define the available commands in the protocol.
const (
	CommandCopy      Command = "COPY"
	CommandCopyAsync Command = "COPY_ASYNC"
	CommandPaste     Command = "PASTE"
	CommandStatus    Command = "STATUS"
)

// Response represents the type of response sent by the server.
type Response string

// Response constants define the possible response types.
const (
	ResponseOK    Response = "OK"
	ResponseError Response = "ERROR"
)

// Request represents a client request with command-specific data.
type Request struct {
	Command Command
	Content []byte
	Size    int
}

// StatusResponse contains information about the daemon's current state.
type StatusResponse struct {
	Uptime         time.Time `json:"uptime"`
	NodeID         string    `json:"node_id"`
	Mode           string    `json:"mode"`
	Version        string    `json:"version"`
	ListenAddr     string    `json:"listen_addr,omitempty"`
	ConnectedPeers []string  `json:"connected_peers"`
	JoinAddresses  []string  `json:"join_addresses"`
	Stats          SyncStats `json:"sync_stats"`
}

// SyncStats contains synchronization statistics.
type SyncStats struct {
	LastSyncTime     string `json:"last_sync_time,omitempty"`
	MessagesSent     uint64 `json:"messages_sent"`
	MessagesReceived uint64 `json:"messages_received"`
	LocalChanges     uint64 `json:"local_changes"`
	RemoteChanges    uint64 `json:"remote_changes"`
}

// ParseRequest parses a command line into a Request.
// Expected format: "COMMAND [size]\n".
func ParseRequest(line string) (*Request, error) {
	if line == "" {
		return nil, fmt.Errorf("empty command")
	}

	// Parse command and optional size
	var cmd string
	var size int
	n, _ := fmt.Sscanf(line, "%s %d", &cmd, &size)
	if n < 1 {
		return nil, fmt.Errorf("invalid command format")
	}

	command := Command(cmd)
	switch command {
	case CommandCopy, CommandCopyAsync:
		if n < 2 || size < 0 {
			return nil, fmt.Errorf("%s requires size parameter", command)
		}
		return &Request{Command: command, Size: size}, nil
	case CommandPaste, CommandStatus:
		return &Request{Command: command}, nil
	default:
		return nil, fmt.Errorf("unknown command: %s", cmd)
	}
}

// FormatResponse formats a response for transmission.
func FormatResponse(resp Response, data any) ([]byte, error) {
	switch resp {
	case ResponseOK:
		switch v := data.(type) {
		case string:
			// For PASTE response
			return []byte(fmt.Sprintf("OK %d\n%s", len(v), v)), nil
		case []byte:
			// For PASTE response (bytes)
			return []byte(fmt.Sprintf("OK %d\n%s", len(v), v)), nil
		case *StatusResponse:
			// For STATUS response
			jsonData, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal status: %w", err)
			}
			return []byte(fmt.Sprintf("STATUS %s\n", jsonData)), nil
		case nil:
			// For COPY response (no data)
			return []byte("OK\n"), nil
		default:
			return nil, fmt.Errorf("unsupported response data type: %T", v)
		}
	case ResponseError:
		if msg, ok := data.(string); ok {
			return []byte(fmt.Sprintf("ERROR %s\n", msg)), nil
		}
		return []byte("ERROR unknown error\n"), nil
	default:
		return nil, fmt.Errorf("unknown response type: %s", resp)
	}
}

// ValidateContent checks if the content size is within acceptable limits.
// This reuses the same limits as the clipboard package.
func ValidateContent(content []byte) error {
	const maxSize = 10 * 1024 * 1024 // 10MB limit
	if len(content) > maxSize {
		return fmt.Errorf("content too large: %d bytes (max: %d)", len(content), maxSize)
	}
	return nil
}
