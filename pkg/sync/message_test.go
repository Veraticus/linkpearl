package sync

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClipboardMessage(t *testing.T) {
	nodeID := "test-node"
	content := "Hello, World!"

	msg := NewClipboardMessage(nodeID, content)

	assert.Equal(t, nodeID, msg.NodeID)
	assert.Equal(t, content, msg.Content)
	assert.Equal(t, computeChecksum(content), msg.Checksum)
	assert.Greater(t, msg.Timestamp, int64(0))
	assert.Equal(t, "1.0", msg.Version)

	// Timestamp should be recent
	msgTime := time.Unix(0, msg.Timestamp)
	assert.WithinDuration(t, time.Now(), msgTime, time.Second)
}

func TestClipboardMessageMarshalUnmarshal(t *testing.T) {
	original := &ClipboardMessage{
		NodeID:    "test-node",
		Timestamp: time.Now().UnixNano(),
		Checksum:  "abc123",
		Content:   "Test content",
		Version:   "1.0",
	}

	// Marshal
	data, err := original.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Unmarshal
	decoded, err := UnmarshalClipboardMessage(data)
	require.NoError(t, err)

	assert.Equal(t, original.NodeID, decoded.NodeID)
	assert.Equal(t, original.Timestamp, decoded.Timestamp)
	assert.Equal(t, original.Checksum, decoded.Checksum)
	assert.Equal(t, original.Content, decoded.Content)
	assert.Equal(t, original.Version, decoded.Version)
}

func TestClipboardMessageValidate(t *testing.T) {
	tests := []struct {
		wantErr error
		msg     *ClipboardMessage
		name    string
	}{
		{
			name: "valid message",
			msg: &ClipboardMessage{
				NodeID:    "test-node",
				Timestamp: time.Now().UnixNano(),
				Checksum:  computeChecksum("valid"),
				Content:   "valid",
				Version:   "1.0",
			},
			wantErr: nil,
		},
		{
			name: "missing node ID",
			msg: &ClipboardMessage{
				NodeID:    "",
				Timestamp: time.Now().UnixNano(),
				Checksum:  "abc",
				Content:   "test",
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "invalid timestamp",
			msg: &ClipboardMessage{
				NodeID:    "test",
				Timestamp: 0,
				Checksum:  "abc",
				Content:   "test",
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "missing checksum",
			msg: &ClipboardMessage{
				NodeID:    "test",
				Timestamp: time.Now().UnixNano(),
				Checksum:  "",
				Content:   "test",
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "checksum mismatch",
			msg: &ClipboardMessage{
				NodeID:    "test",
				Timestamp: time.Now().UnixNano(),
				Checksum:  "wrong-checksum",
				Content:   "test",
			},
			wantErr: ErrChecksumMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClipboardMessageTime(t *testing.T) {
	now := time.Now()
	msg := &ClipboardMessage{
		Timestamp: now.UnixNano(),
	}

	msgTime := msg.Time()
	assert.Equal(t, now.Unix(), msgTime.Unix())
	// Nanosecond precision might be slightly off due to conversion
	assert.WithinDuration(t, now, msgTime, time.Microsecond)
}

func TestComputeChecksum(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{
			content:  "",
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			content:  "Hello, World!",
			expected: "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f",
		},
		{
			content:  "The quick brown fox jumps over the lazy dog",
			expected: "d7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592",
		},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			checksum := computeChecksum(tt.content)
			assert.Equal(t, tt.expected, checksum)

			// Should be consistent
			checksum2 := computeChecksum(tt.content)
			assert.Equal(t, checksum, checksum2)
		})
	}
}

func TestUnmarshalClipboardMessageErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "invalid JSON",
			data: []byte("{invalid json}"),
		},
		{
			name: "empty data",
			data: []byte(""),
		},
		{
			name: "wrong type",
			data: []byte(`"string instead of object"`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := UnmarshalClipboardMessage(tt.data)
			assert.Error(t, err)
			assert.Nil(t, msg)
		})
	}
}
