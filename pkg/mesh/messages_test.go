package mesh

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test message marshaling and unmarshaling
func TestMarshalUnmarshalMessage(t *testing.T) {
	tests := []struct {
		name    string
		msgType MessageType
		from    string
		payload interface{}
		wantErr bool
	}{
		{
			name:    "peer list message",
			msgType: MessageTypePeerList,
			from:    "node1",
			payload: PeerListMessage{
				Peers: []Node{
					{ID: "node2", Mode: "full", Addr: "192.168.1.2:8080"},
					{ID: "node3", Mode: "client"},
				},
			},
		},
		{
			name:    "ping message",
			msgType: MessageTypePing,
			from:    "node2",
			payload: PingMessage{
				Timestamp: time.Now().Unix(),
			},
		},
		{
			name:    "pong message",
			msgType: MessageTypePong,
			from:    "node3",
			payload: PongMessage{
				Timestamp: time.Now().Unix(),
			},
		},
		{
			name:    "clipboard message",
			msgType: MessageTypeClipboard,
			from:    "node4",
			payload: map[string]string{
				"content": "test clipboard data",
				"format":  "text/plain",
			},
		},
		{
			name:    "empty payload",
			msgType: MessageTypePing,
			from:    "node5",
			payload: struct{}{},
		},
		{
			name:    "nil payload",
			msgType: MessageTypePing,
			from:    "node6",
			payload: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal the message
			data, err := marshalMessage(tt.msgType, tt.from, tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Unmarshal the message
			msgType, from, payload, err := unmarshalMessage(data)
			require.NoError(t, err)

			// Verify message type and from
			assert.Equal(t, tt.msgType, msgType)
			assert.Equal(t, tt.from, from)

			// Verify payload by re-marshaling and comparing
			if tt.payload != nil {
				originalPayload, _ := json.Marshal(tt.payload)
				assert.Equal(t, string(originalPayload), string(payload))
			}
		})
	}
}

// Test unmarshaling invalid messages
func TestUnmarshalInvalidMessage(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "invalid JSON",
			data:    []byte("{invalid json}"),
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte(""),
			wantErr: true,
		},
		{
			name:    "missing type field",
			data:    []byte(`{"from":"node1","payload":{}}`),
			wantErr: false, // Will succeed but type will be empty
		},
		{
			name:    "missing from field",
			data:    []byte(`{"type":"ping","payload":{}}`),
			wantErr: false, // Will succeed but from will be empty
		},
		{
			name:    "null data",
			data:    []byte("null"),
			wantErr: false, // JSON unmarshal accepts null
		},
		{
			name:    "array instead of object",
			data:    []byte(`["not","an","object"]`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := unmarshalMessage(tt.data)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test message handler registration and handling
func TestMessageHandler(t *testing.T) {
	h := newMessageHandler()

	// Test registering handlers
	var called1, called2 bool
	h.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		called1 = true
		return nil
	})

	h.Register(MessageTypePong, func(from string, payload json.RawMessage) error {
		called2 = true
		return nil
	})

	// Test handling registered message type
	err := h.Handle(MessageTypePing, "node1", json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.True(t, called1, "Ping handler not called")
	assert.False(t, called2, "Pong handler should not be called")

	// Reset flags
	called1, called2 = false, false

	// Test handling different message type
	err = h.Handle(MessageTypePong, "node2", json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.False(t, called1, "Ping handler should not be called")
	assert.True(t, called2, "Pong handler not called")

	// Test handling unregistered message type
	err = h.Handle(MessageTypeClipboard, "node3", json.RawMessage(`{}`))
	assert.Error(t, err, "Handle() should error for unregistered message type")
}

// Test handler error propagation
func TestMessageHandlerError(t *testing.T) {
	h := newMessageHandler()

	expectedErr := errors.New("handler error")
	h.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		return expectedErr
	})

	err := h.Handle(MessageTypePing, "node1", json.RawMessage(`{}`))
	assert.Equal(t, expectedErr, err)
}

// Test handler payload parsing
func TestMessageHandlerPayloadParsing(t *testing.T) {
	h := newMessageHandler()

	// Register handler that parses PingMessage
	var receivedPing PingMessage
	h.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		return json.Unmarshal(payload, &receivedPing)
	})

	// Send ping message
	pingMsg := PingMessage{Timestamp: 12345}
	payload, _ := json.Marshal(pingMsg)
	
	err := h.Handle(MessageTypePing, "node1", payload)
	assert.NoError(t, err)
	assert.Equal(t, pingMsg.Timestamp, receivedPing.Timestamp)
}

// Test message router SendToPeer
func TestMessageRouterSendToPeer(t *testing.T) {
	var sentNodeID string
	var sentData []byte
	var sendErr error

	sendFunc := func(nodeID string, data []byte) error {
		sentNodeID = nodeID
		sentData = data
		return sendErr
	}

	router := newMessageRouter("local-node", sendFunc)

	// Test successful send
	payload := PingMessage{Timestamp: 12345}
	err := router.SendToPeer("remote-node", MessageTypePing, payload)
	assert.NoError(t, err)
	assert.Equal(t, "remote-node", sentNodeID)

	// Verify message structure
	msgType, from, msgPayload, err := unmarshalMessage(sentData)
	require.NoError(t, err, "Failed to unmarshal sent message")
	assert.Equal(t, MessageTypePing, msgType)
	assert.Equal(t, "local-node", from)

	var receivedPing PingMessage
	err = json.Unmarshal(msgPayload, &receivedPing)
	require.NoError(t, err, "Failed to unmarshal ping payload")
	assert.Equal(t, payload.Timestamp, receivedPing.Timestamp)

	// Test send error
	sendErr = errors.New("send failed")
	err = router.SendToPeer("remote-node", MessageTypePing, payload)
	assert.Equal(t, sendErr, err)
}

// Test message router Broadcast
func TestMessageRouterBroadcast(t *testing.T) {
	var sentNodeID string
	var sentData []byte

	sendFunc := func(nodeID string, data []byte) error {
		sentNodeID = nodeID
		sentData = data
		return nil
	}

	router := newMessageRouter("local-node", sendFunc)

	// Test broadcast
	payload := PeerListMessage{
		Peers: []Node{
			{ID: "node1", Mode: "full", Addr: "192.168.1.1:8080"},
			{ID: "node2", Mode: "client"},
		},
	}

	err := router.Broadcast(MessageTypePeerList, payload)
	assert.NoError(t, err)
	assert.Empty(t, sentNodeID)

	// Verify message structure
	msgType, from, msgPayload, err := unmarshalMessage(sentData)
	require.NoError(t, err, "Failed to unmarshal sent message")
	assert.Equal(t, MessageTypePeerList, msgType)
	assert.Equal(t, "local-node", from)

	var receivedPeerList PeerListMessage
	err = json.Unmarshal(msgPayload, &receivedPeerList)
	require.NoError(t, err, "Failed to unmarshal peer list payload")
	assert.Len(t, receivedPeerList.Peers, len(payload.Peers))
}

// Test message router ProcessMessage
func TestMessageRouterProcessMessage(t *testing.T) {
	router := newMessageRouter("local-node", nil)

	// Register handlers
	var handledPing bool
	var handledFrom string
	var handledPayload json.RawMessage

	router.handler.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		handledPing = true
		handledFrom = from
		handledPayload = payload
		return nil
	})

	// Create a test message
	pingPayload := PingMessage{Timestamp: 12345}
	msgData, err := marshalMessage(MessageTypePing, "remote-node", pingPayload)
	require.NoError(t, err, "Failed to marshal test message")

	// Process the message
	err = router.ProcessMessage(msgData)
	assert.NoError(t, err)
	assert.True(t, handledPing, "Ping handler not called")
	assert.Equal(t, "remote-node", handledFrom)

	var receivedPing PingMessage
	err = json.Unmarshal(handledPayload, &receivedPing)
	require.NoError(t, err, "Failed to unmarshal handled payload")
	assert.Equal(t, pingPayload.Timestamp, receivedPing.Timestamp)

	// Test processing invalid message
	err = router.ProcessMessage([]byte("invalid json"))
	assert.Error(t, err, "ProcessMessage() should error on invalid JSON")

	// Test processing message with no handler
	clipboardData, _ := marshalMessage(MessageTypeClipboard, "remote-node", map[string]string{"data": "test"})
	err = router.ProcessMessage(clipboardData)
	assert.Error(t, err, "ProcessMessage() should error for unregistered message type")
}

// Test concurrent message handling
func TestMessageRouterConcurrent(t *testing.T) {
	const numGoroutines = 100
	const numMessages = 10

	var sendCount atomic.Int32
	sendFunc := func(nodeID string, data []byte) error {
		sendCount.Add(1)
		// Simulate some processing time
		time.Sleep(time.Microsecond)
		return nil
	}

	router := newMessageRouter("local-node", sendFunc)

	// Register a handler that counts messages
	var handleCount atomic.Int32
	router.handler.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		handleCount.Add(1)
		// Simulate some processing time
		time.Sleep(time.Microsecond)
		return nil
	})

	// Start goroutines to send messages
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Goroutines for sending
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				payload := PingMessage{Timestamp: int64(id*1000 + j)}
				err := router.SendToPeer("remote-node", MessageTypePing, payload)
				assert.NoError(t, err)
			}
		}(i)
	}

	// Goroutines for processing
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				payload := PingMessage{Timestamp: int64(id*1000 + j)}
				msgData, _ := marshalMessage(MessageTypePing, "remote-node", payload)
				err := router.ProcessMessage(msgData)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	expectedSends := int32(numGoroutines * numMessages)
	assert.Equal(t, expectedSends, sendCount.Load())

	expectedHandles := int32(numGoroutines * numMessages)
	assert.Equal(t, expectedHandles, handleCount.Load())
}

// Test handler overwriting
func TestMessageHandlerOverwrite(t *testing.T) {
	h := newMessageHandler()

	// Register first handler
	var handler1Called bool
	h.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		handler1Called = true
		return nil
	})

	// Register second handler for same type (should overwrite)
	var handler2Called bool
	h.Register(MessageTypePing, func(from string, payload json.RawMessage) error {
		handler2Called = true
		return nil
	})

	// Handle message
	err := h.Handle(MessageTypePing, "node1", json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.False(t, handler1Called, "First handler should not be called after overwrite")
	assert.True(t, handler2Called, "Second handler should be called")
}

// Test different message types with router
func TestMessageRouterAllMessageTypes(t *testing.T) {
	var sentMessages [][]byte
	sendFunc := func(nodeID string, data []byte) error {
		sentMessages = append(sentMessages, data)
		return nil
	}

	router := newMessageRouter("local-node", sendFunc)

	// Test all message types
	tests := []struct {
		name    string
		msgType MessageType
		payload interface{}
	}{
		{
			name:    "peer list",
			msgType: MessageTypePeerList,
			payload: PeerListMessage{
				Peers: []Node{
					{ID: "node1", Mode: "full", Addr: "192.168.1.1:8080"},
				},
			},
		},
		{
			name:    "ping",
			msgType: MessageTypePing,
			payload: PingMessage{Timestamp: 12345},
		},
		{
			name:    "pong",
			msgType: MessageTypePong,
			payload: PongMessage{Timestamp: 67890},
		},
		{
			name:    "clipboard",
			msgType: MessageTypeClipboard,
			payload: map[string]interface{}{
				"content": "test data",
				"format":  "text/plain",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sentMessages = nil

			// Send the message
			err := router.SendToPeer("target-node", tt.msgType, tt.payload)
			assert.NoError(t, err)
			require.Len(t, sentMessages, 1, "Expected 1 message sent")

			// Verify the sent message
			msgType, from, payload, err := unmarshalMessage(sentMessages[0])
			require.NoError(t, err, "Failed to unmarshal sent message")
			assert.Equal(t, tt.msgType, msgType)
			assert.Equal(t, "local-node", from)

			// Verify payload is valid JSON
			var decoded interface{}
			err = json.Unmarshal(payload, &decoded)
			assert.NoError(t, err, "Invalid payload JSON")
		})
	}
}

// Test edge case: very large payload
func TestMessageRouterLargePayload(t *testing.T) {
	var sentData []byte
	sendFunc := func(nodeID string, data []byte) error {
		sentData = data
		return nil
	}

	router := newMessageRouter("local-node", sendFunc)

	// Create a large payload
	largeData := make([]byte, 1024*1024) // 1MB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	payload := map[string]interface{}{
		"data": largeData,
		"size": len(largeData),
	}

	err := router.SendToPeer("remote-node", MessageTypeClipboard, payload)
	assert.NoError(t, err)

	// Verify we can unmarshal the large message
	msgType, from, msgPayload, err := unmarshalMessage(sentData)
	require.NoError(t, err, "Failed to unmarshal large message")
	assert.Equal(t, MessageTypeClipboard, msgType)
	assert.Equal(t, "local-node", from)

	// Verify payload integrity
	var decoded map[string]interface{}
	err = json.Unmarshal(msgPayload, &decoded)
	require.NoError(t, err, "Failed to unmarshal payload")
	assert.Equal(t, float64(len(largeData)), decoded["size"].(float64))
}

// Test edge case: empty string fields
func TestMessageRouterEmptyFields(t *testing.T) {
	var sentData []byte
	sendFunc := func(nodeID string, data []byte) error {
		sentData = data
		return nil
	}

	// Test with empty local node ID
	router := newMessageRouter("", sendFunc)

	err := router.SendToPeer("remote-node", MessageTypePing, PingMessage{Timestamp: 12345})
	assert.NoError(t, err)

	msgType, from, _, err := unmarshalMessage(sentData)
	require.NoError(t, err, "Failed to unmarshal message")
	assert.Empty(t, from)
	assert.Equal(t, MessageTypePing, msgType)

	// Test sending to empty node ID
	err = router.SendToPeer("", MessageTypePing, PingMessage{Timestamp: 12345})
	assert.NoError(t, err)
}

// Test payload type edge cases
func TestMarshalMessagePayloadTypes(t *testing.T) {
	tests := []struct {
		name    string
		payload interface{}
		wantErr bool
	}{
		{
			name:    "string payload",
			payload: "simple string",
		},
		{
			name:    "number payload",
			payload: 12345,
		},
		{
			name:    "boolean payload",
			payload: true,
		},
		{
			name:    "array payload",
			payload: []string{"a", "b", "c"},
		},
		{
			name:    "nested struct",
			payload: struct {
				A string
				B struct {
					C int
					D bool
				}
			}{
				A: "test",
				B: struct {
					C int
					D bool
				}{C: 42, D: true},
			},
		},
		{
			name:    "channel (non-marshalable)",
			payload: make(chan int),
			wantErr: true,
		},
		{
			name:    "function (non-marshalable)",
			payload: func() {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := marshalMessage(MessageTypePing, "node1", tt.payload)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}