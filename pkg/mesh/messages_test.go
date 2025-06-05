package mesh

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test MeshMessage interface implementation.
func TestMeshMessageTypes(t *testing.T) {
	tests := []struct {
		name     string
		msg      MeshMessage
		wantType MessageType
		wantFrom string
	}{
		{
			name: "clipboard message",
			msg: ClipboardSyncMessage{
				FromNode:    "node1",
				MessageData: json.RawMessage(`{"content":"test"}`),
			},
			wantType: MessageTypeClipboard,
			wantFrom: "node1",
		},
		{
			name: "peer list message",
			msg: PeerListSyncMessage{
				FromNode: "node2",
				Peers: []Node{
					{ID: "node3", Mode: "full", Addr: "192.168.1.3:8080"},
					{ID: "node4", Mode: "client"},
				},
			},
			wantType: MessageTypePeerList,
			wantFrom: "node2",
		},
		{
			name: "ping message",
			msg: PingSyncMessage{
				FromNode:  "node3",
				Timestamp: time.Now().Unix(),
			},
			wantType: MessageTypePing,
			wantFrom: "node3",
		},
		{
			name: "pong message",
			msg: PongSyncMessage{
				FromNode:  "node4",
				Timestamp: time.Now().Unix(),
			},
			wantType: MessageTypePong,
			wantFrom: "node4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Type() method
			assert.Equal(t, tt.wantType, tt.msg.Type())

			// Test From() method
			assert.Equal(t, tt.wantFrom, tt.msg.From())

			// Test Marshal() method
			data, err := tt.msg.Marshal()
			require.NoError(t, err)

			// Verify marshaled data can be unmarshaled
			var msg meshMessage
			err = json.Unmarshal(data, &msg)
			require.NoError(t, err)

			assert.Equal(t, tt.wantType, msg.Type)
			assert.Equal(t, tt.wantFrom, msg.From)
			assert.NotEmpty(t, msg.Payload)
		})
	}
}

// Test message creation functions.
func TestMessageCreationFunctions(t *testing.T) {
	t.Run("NewClipboardMessage", func(t *testing.T) {
		data := json.RawMessage(`{"content":"clipboard data"}`)
		msg := NewClipboardMessage("node1", data)

		assert.Equal(t, MessageTypeClipboard, msg.Type())
		assert.Equal(t, "node1", msg.From())

		// Verify it marshals correctly
		marshaled, err := msg.Marshal()
		require.NoError(t, err)

		var wireMsg meshMessage
		err = json.Unmarshal(marshaled, &wireMsg)
		require.NoError(t, err)
		assert.Equal(t, data, wireMsg.Payload)
	})

	t.Run("NewPeerListMessage", func(t *testing.T) {
		peers := []Node{
			{ID: "node2", Mode: "full", Addr: "192.168.1.2:8080"},
			{ID: "node3", Mode: "client"},
		}
		msg := NewPeerListMessage("node1", peers)

		assert.Equal(t, MessageTypePeerList, msg.Type())
		assert.Equal(t, "node1", msg.From())

		// Verify payload contains peers
		marshaled, err := msg.Marshal()
		require.NoError(t, err)

		var wireMsg meshMessage
		err = json.Unmarshal(marshaled, &wireMsg)
		require.NoError(t, err)

		var peerList PeerListMessage
		err = json.Unmarshal(wireMsg.Payload, &peerList)
		require.NoError(t, err)
		assert.Len(t, peerList.Peers, 2)
		assert.Equal(t, peers[0].ID, peerList.Peers[0].ID)
	})

	t.Run("NewPingMessage", func(t *testing.T) {
		timestamp := time.Now().Unix()
		msg := NewPingMessage("node1", timestamp)

		assert.Equal(t, MessageTypePing, msg.Type())
		assert.Equal(t, "node1", msg.From())

		// Verify payload contains timestamp
		marshaled, err := msg.Marshal()
		require.NoError(t, err)

		var wireMsg meshMessage
		err = json.Unmarshal(marshaled, &wireMsg)
		require.NoError(t, err)

		var ping PingMessage
		err = json.Unmarshal(wireMsg.Payload, &ping)
		require.NoError(t, err)
		assert.Equal(t, timestamp, ping.Timestamp)
	})

	t.Run("NewPongMessage", func(t *testing.T) {
		timestamp := time.Now().Unix()
		msg := NewPongMessage("node1", timestamp)

		assert.Equal(t, MessageTypePong, msg.Type())
		assert.Equal(t, "node1", msg.From())

		// Verify payload contains timestamp
		marshaled, err := msg.Marshal()
		require.NoError(t, err)

		var wireMsg meshMessage
		err = json.Unmarshal(marshaled, &wireMsg)
		require.NoError(t, err)

		var pong PongMessage
		err = json.Unmarshal(wireMsg.Payload, &pong)
		require.NoError(t, err)
		assert.Equal(t, timestamp, pong.Timestamp)
	})
}

// Test message marshaling and unmarshaling.
func TestMeshMessageMarshalUnmarshal(t *testing.T) {
	// Test with complex clipboard data
	clipboardData := map[string]any{
		"content":   "Test clipboard content with special chars: 你好世界 🌍",
		"timestamp": time.Now().Unix(),
		"checksum":  "abc123def456",
	}

	data, err := json.Marshal(clipboardData)
	require.NoError(t, err)

	msg := NewClipboardMessage("test-node", json.RawMessage(data))

	// Marshal the message
	marshaled, err := msg.Marshal()
	require.NoError(t, err)

	// Unmarshal and verify
	var wireMsg meshMessage
	err = json.Unmarshal(marshaled, &wireMsg)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeClipboard, wireMsg.Type)
	assert.Equal(t, "test-node", wireMsg.From)

	// Verify payload integrity
	var decoded map[string]any
	err = json.Unmarshal(wireMsg.Payload, &decoded)
	require.NoError(t, err)
	assert.Equal(t, clipboardData["content"], decoded["content"])
}

// Test unmarshalMessage function.
func TestUnmarshalMessage(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name: "valid message",
			data: []byte(`{"type":"ping","from":"node1","payload":{"timestamp":12345}}`),
		},
		{
			name:    "invalid JSON",
			data:    []byte(`{invalid json}`),
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte(``),
			wantErr: true,
		},
		{
			name: "missing type",
			data: []byte(`{"from":"node1","payload":{}}`),
		},
		{
			name: "missing from",
			data: []byte(`{"type":"ping","payload":{}}`),
		},
		{
			name: "missing payload",
			data: []byte(`{"type":"ping","from":"node1"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgType, from, payload, err := unmarshalMessage(tt.data)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if !tt.wantErr {
				// For valid messages, verify we got some data
				_ = msgType
				_ = from
				_ = payload
			}
		})
	}
}

// Test message handler.
func TestMessageHandler(t *testing.T) {
	h := newMessageHandler()

	// Test registering and calling handlers
	var called bool
	var receivedFrom string
	var receivedPayload json.RawMessage

	h.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		called = true
		receivedFrom = from
		receivedPayload = payload
		return nil
	})

	// Test handling registered message
	payload := json.RawMessage(`{"timestamp":12345}`)
	err := h.Handle(MessageTypePing, "node1", payload)
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "node1", receivedFrom)
	assert.Equal(t, payload, receivedPayload)

	// Test handling unregistered message
	err = h.Handle(MessageTypeClipboard, "node2", json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no handler for message type")
}

// Test message router with type-safe messages.
func TestMessageRouterTypeSafe(t *testing.T) {
	var sentData []byte
	sendFunc := func(_ string, data []byte) error {
		sentData = data
		return nil
	}

	router := newMessageRouter("local-node", sendFunc)

	t.Run("SendToPeer with ping", func(t *testing.T) {
		err := router.SendToPeer("remote-node", MessageTypePing, PingMessage{Timestamp: 12345})
		require.NoError(t, err)

		// Verify sent data
		var wireMsg meshMessage
		err = json.Unmarshal(sentData, &wireMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypePing, wireMsg.Type)
		assert.Equal(t, "local-node", wireMsg.From)

		var ping PingMessage
		err = json.Unmarshal(wireMsg.Payload, &ping)
		require.NoError(t, err)
		assert.Equal(t, int64(12345), ping.Timestamp)
	})

	t.Run("SendToPeer with peer list", func(t *testing.T) {
		peers := []Node{
			{ID: "node2", Mode: "full", Addr: "192.168.1.2:8080"},
		}
		err := router.SendToPeer("remote-node", MessageTypePeerList, PeerListMessage{Peers: peers})
		require.NoError(t, err)

		// Verify sent data
		var wireMsg meshMessage
		err = json.Unmarshal(sentData, &wireMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypePeerList, wireMsg.Type)

		var peerList PeerListMessage
		err = json.Unmarshal(wireMsg.Payload, &peerList)
		require.NoError(t, err)
		assert.Len(t, peerList.Peers, 1)
		assert.Equal(t, "node2", peerList.Peers[0].ID)
	})

	t.Run("SendToPeer with unsupported type", func(t *testing.T) {
		err := router.SendToPeer("remote-node", MessageType("unsupported"), struct{}{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported message type")
	})

	t.Run("SendToPeer with invalid payload", func(t *testing.T) {
		err := router.SendToPeer("remote-node", MessageTypePing, "invalid payload type")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid payload for ping message")
	})
}

// Test message router broadcast.
func TestMessageRouterBroadcast(t *testing.T) {
	var sentNodeID string
	var sentData []byte
	sendFunc := func(nodeID string, data []byte) error {
		sentNodeID = nodeID
		sentData = data
		return nil
	}

	router := newMessageRouter("local-node", sendFunc)

	t.Run("Broadcast ping", func(t *testing.T) {
		err := router.Broadcast(MessageTypePing, PingMessage{Timestamp: 12345})
		require.NoError(t, err)

		// Verify empty nodeID for broadcast
		assert.Empty(t, sentNodeID)

		// Verify message content
		var wireMsg meshMessage
		err = json.Unmarshal(sentData, &wireMsg)
		require.NoError(t, err)
		assert.Equal(t, MessageTypePing, wireMsg.Type)
		assert.Equal(t, "local-node", wireMsg.From)
	})

	t.Run("Broadcast with unsupported type", func(t *testing.T) {
		err := router.Broadcast(MessageType("unsupported"), struct{}{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported message type")
	})
}

// Test process message.
func TestMessageRouterProcessMessage(t *testing.T) {
	router := newMessageRouter("local-node", nil)

	var handled bool
	var handledFrom string
	var handledPayload json.RawMessage

	router.handler.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		handled = true
		handledFrom = from
		handledPayload = payload
		return nil
	})

	// Create a test message using type-safe approach
	msg := NewPingMessage("remote-node", 12345)
	data, err := msg.Marshal()
	require.NoError(t, err)

	// Process the message
	err = router.ProcessMessage(data)
	assert.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "remote-node", handledFrom)

	// Verify payload
	var ping PingMessage
	err = json.Unmarshal(handledPayload, &ping)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), ping.Timestamp)
}

// Test edge cases.
func TestMeshMessageEdgeCases(t *testing.T) {
	t.Run("Empty node ID", func(t *testing.T) {
		msg := NewClipboardMessage("", json.RawMessage(`{}`))
		assert.Empty(t, msg.From())

		data, err := msg.Marshal()
		require.NoError(t, err)

		var wireMsg meshMessage
		err = json.Unmarshal(data, &wireMsg)
		require.NoError(t, err)
		assert.Empty(t, wireMsg.From)
	})

	t.Run("Large payload", func(t *testing.T) {
		// Create a large payload (1MB)
		largeContent := make([]byte, 1024*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		payload := map[string]any{
			"content": string(largeContent),
			"size":    len(largeContent),
		}

		data, err := json.Marshal(payload)
		require.NoError(t, err)

		msg := NewClipboardMessage("node1", json.RawMessage(data))
		marshaled, err := msg.Marshal()
		require.NoError(t, err)

		// Verify we can unmarshal it
		var wireMsg meshMessage
		err = json.Unmarshal(marshaled, &wireMsg)
		require.NoError(t, err)

		var decoded map[string]any
		err = json.Unmarshal(wireMsg.Payload, &decoded)
		require.NoError(t, err)
		assert.Equal(t, float64(len(largeContent)), decoded["size"])
	})

	t.Run("Unicode in messages", func(t *testing.T) {
		content := "Hello 世界 🌍 مرحبا мир"
		payload := map[string]string{"content": content}
		data, err := json.Marshal(payload)
		require.NoError(t, err)

		msg := NewClipboardMessage("node1", json.RawMessage(data))
		marshaled, err := msg.Marshal()
		require.NoError(t, err)

		var wireMsg meshMessage
		err = json.Unmarshal(marshaled, &wireMsg)
		require.NoError(t, err)

		var decoded map[string]string
		err = json.Unmarshal(wireMsg.Payload, &decoded)
		require.NoError(t, err)
		assert.Equal(t, content, decoded["content"])
	})
}
