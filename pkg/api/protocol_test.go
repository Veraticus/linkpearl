package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		want    *Request
		name    string
		input   string
		errMsg  string
		wantErr bool
	}{
		{
			name:  "copy command with size",
			input: "COPY 13",
			want: &Request{
				Command: CommandCopy,
				Size:    13,
			},
			wantErr: false,
		},
		{
			name:  "paste command",
			input: "PASTE",
			want: &Request{
				Command: CommandPaste,
			},
			wantErr: false,
		},
		{
			name:  "status command",
			input: "STATUS",
			want: &Request{
				Command: CommandStatus,
			},
			wantErr: false,
		},
		{
			name:    "copy without size",
			input:   "COPY",
			want:    nil,
			wantErr: true,
			errMsg:  "COPY requires size parameter",
		},
		{
			name:    "copy with negative size",
			input:   "COPY -5",
			want:    nil,
			wantErr: true,
			errMsg:  "COPY requires size parameter",
		},
		{
			name:    "unknown command",
			input:   "UNKNOWN",
			want:    nil,
			wantErr: true,
			errMsg:  "unknown command: UNKNOWN",
		},
		{
			name:    "empty command",
			input:   "",
			want:    nil,
			wantErr: true,
			errMsg:  "empty command",
		},
		{
			name:  "command with extra spaces",
			input: "  PASTE  ",
			want: &Request{
				Command: CommandPaste,
			},
			wantErr: false,
		},
		{
			name:  "copy with extra parameters ignored",
			input: "COPY 10 extra",
			want: &Request{
				Command: CommandCopy,
				Size:    10,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRequest(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseRequest() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("ParseRequest() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseRequest() unexpected error = %v", err)
				return
			}
			if got.Command != tt.want.Command {
				t.Errorf("ParseRequest() Command = %v, want %v", got.Command, tt.want.Command)
			}
			if got.Size != tt.want.Size {
				t.Errorf("ParseRequest() Size = %v, want %v", got.Size, tt.want.Size)
			}
		})
	}
}

func TestFormatResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    Response
		data    any
		want    string
		wantErr bool
	}{
		{
			name:    "OK response for COPY",
			resp:    ResponseOK,
			data:    nil,
			want:    "OK\n",
			wantErr: false,
		},
		{
			name:    "OK response for PASTE with string",
			resp:    ResponseOK,
			data:    "Hello, World!",
			want:    "OK 13\nHello, World!",
			wantErr: false,
		},
		{
			name:    "OK response for PASTE with bytes",
			resp:    ResponseOK,
			data:    []byte("Hello, World!"),
			want:    "OK 13\nHello, World!",
			wantErr: false,
		},
		{
			name:    "OK response for PASTE empty",
			resp:    ResponseOK,
			data:    "",
			want:    "OK 0\n",
			wantErr: false,
		},
		{
			name: "STATUS response",
			resp: ResponseOK,
			data: &StatusResponse{
				NodeID:  "test-node",
				Mode:    "full",
				Version: "1.0.0",
				Uptime:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			want:    "STATUS ",
			wantErr: false,
		},
		{
			name:    "ERROR response",
			resp:    ResponseError,
			data:    "test error",
			want:    "ERROR test error\n",
			wantErr: false,
		},
		{
			name:    "ERROR response with nil",
			resp:    ResponseError,
			data:    nil,
			want:    "ERROR unknown error\n",
			wantErr: false,
		},
		{
			name:    "unsupported response type",
			resp:    Response("UNKNOWN"),
			data:    nil,
			want:    "",
			wantErr: true,
		},
		{
			name:    "unsupported data type for OK",
			resp:    ResponseOK,
			data:    123, // int is not supported
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatResponse(tt.resp, tt.data)
			if tt.wantErr {
				if err == nil {
					t.Errorf("FormatResponse() error = nil, wantErr %v", tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("FormatResponse() unexpected error = %v", err)
				return
			}

			gotStr := string(got)

			// For STATUS responses, just check the prefix since JSON can vary
			if strings.HasPrefix(tt.want, "STATUS ") {
				if !strings.HasPrefix(gotStr, "STATUS ") {
					t.Errorf("FormatResponse() = %v, want prefix %v", gotStr, tt.want)
				}
				// Verify it's valid JSON
				jsonPart := strings.TrimPrefix(gotStr, "STATUS ")
				jsonPart = strings.TrimSuffix(jsonPart, "\n")
				var status StatusResponse
				if err := json.Unmarshal([]byte(jsonPart), &status); err != nil {
					t.Errorf("FormatResponse() produced invalid JSON: %v", err)
				}
			} else if gotStr != tt.want {
				t.Errorf("FormatResponse() = %v, want %v", gotStr, tt.want)
			}
		})
	}
}

func TestValidateContent(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		content []byte
		wantErr bool
	}{
		{
			name:    "valid small content",
			content: []byte("Hello, World!"),
			wantErr: false,
		},
		{
			name:    "empty content",
			content: []byte{},
			wantErr: false,
		},
		{
			name:    "exactly at limit",
			content: make([]byte, 10*1024*1024),
			wantErr: false,
		},
		{
			name:    "over limit",
			content: make([]byte, 10*1024*1024+1),
			wantErr: true,
			errMsg:  "content too large: 10485761 bytes (max: 10485760)",
		},
		{
			name:    "nil content",
			content: nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContent(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateContent() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("ValidateContent() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("ValidateContent() unexpected error = %v", err)
			}
		})
	}
}

func TestStatusResponseJSON(t *testing.T) {
	// Test that StatusResponse can be properly marshaled/unmarshaled
	status := &StatusResponse{
		NodeID:         "test-node-123",
		Mode:           "full",
		Version:        "1.0.0",
		Uptime:         time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		ListenAddr:     ":8080",
		ConnectedPeers: []string{"peer1", "peer2"},
		JoinAddresses:  []string{"host1:8080", "host2:8080"},
		Stats: SyncStats{
			MessagesSent:     100,
			MessagesReceived: 200,
			LocalChanges:     10,
			RemoteChanges:    20,
			LastSyncTime:     "2024-01-15T10:29:00Z",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal StatusResponse: %v", err)
	}

	// Unmarshal back
	var decoded StatusResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal StatusResponse: %v", err)
	}

	// Verify fields
	if decoded.NodeID != status.NodeID {
		t.Errorf("NodeID = %v, want %v", decoded.NodeID, status.NodeID)
	}
	if decoded.Mode != status.Mode {
		t.Errorf("Mode = %v, want %v", decoded.Mode, status.Mode)
	}
	if decoded.Version != status.Version {
		t.Errorf("Version = %v, want %v", decoded.Version, status.Version)
	}
	if !decoded.Uptime.Equal(status.Uptime) {
		t.Errorf("Uptime = %v, want %v", decoded.Uptime, status.Uptime)
	}
	if decoded.ListenAddr != status.ListenAddr {
		t.Errorf("ListenAddr = %v, want %v", decoded.ListenAddr, status.ListenAddr)
	}
	if len(decoded.ConnectedPeers) != len(status.ConnectedPeers) {
		t.Errorf("ConnectedPeers length = %v, want %v", len(decoded.ConnectedPeers), len(status.ConnectedPeers))
	}
	if decoded.Stats.MessagesSent != status.Stats.MessagesSent {
		t.Errorf("Stats.MessagesSent = %v, want %v", decoded.Stats.MessagesSent, status.Stats.MessagesSent)
	}
}
