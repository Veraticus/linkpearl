package mesh

import (
	"encoding/json"
	"fmt"
)

// MessageType identifies the type of mesh message
type MessageType string

const (
	// MessageTypePeerList is sent to share the list of connected peers
	MessageTypePeerList MessageType = "peer_list"

	// MessageTypeClipboard is for clipboard sync messages
	MessageTypeClipboard MessageType = "clipboard"

	// MessageTypePing is for keepalive/health checks
	MessageTypePing MessageType = "ping"

	// MessageTypePong is the response to ping
	MessageTypePong MessageType = "pong"
)

// meshMessage is the base structure for all mesh messages
type meshMessage struct {
	Type    MessageType     `json:"type"`
	From    string          `json:"from"`
	Payload json.RawMessage `json:"payload"`
}

// PeerListMessage contains the list of connected peers
type PeerListMessage struct {
	Peers []Node `json:"peers"`
}

// PingMessage is sent for keepalive
type PingMessage struct {
	Timestamp int64 `json:"timestamp"`
}

// PongMessage is the response to a ping
type PongMessage struct {
	Timestamp int64 `json:"timestamp"`
}

// marshalMessage creates a mesh message with the given type and payload
func marshalMessage(msgType MessageType, from string, payload interface{}) ([]byte, error) {
	// Marshal the payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create the mesh message
	msg := meshMessage{
		Type:    msgType,
		From:    from,
		Payload: payloadBytes,
	}

	// Marshal the complete message
	return json.Marshal(msg)
}

// unmarshalMessage extracts the message type and payload
func unmarshalMessage(data []byte) (MessageType, string, json.RawMessage, error) {
	var msg meshMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", "", nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return msg.Type, msg.From, msg.Payload, nil
}

// messageHandler processes incoming messages based on their type
type messageHandler struct {
	handlers map[MessageType]func(from string, payload json.RawMessage) error
}

// newMessageHandler creates a new message handler
func newMessageHandler() *messageHandler {
	return &messageHandler{
		handlers: make(map[MessageType]func(from string, payload json.RawMessage) error),
	}
}

// Register registers a handler for a message type
func (h *messageHandler) Register(msgType MessageType, handler func(from string, payload json.RawMessage) error) {
	h.handlers[msgType] = handler
}

// Handle processes a message
func (h *messageHandler) Handle(msgType MessageType, from string, payload json.RawMessage) error {
	handler, exists := h.handlers[msgType]
	if !exists {
		return fmt.Errorf("no handler for message type: %s", msgType)
	}

	return handler(from, payload)
}

// messageRouter routes messages between peers and local handlers
type messageRouter struct {
	localNode string
	handler   *messageHandler
	sendFunc  func(nodeID string, data []byte) error
}

// newMessageRouter creates a new message router
func newMessageRouter(localNode string, sendFunc func(nodeID string, data []byte) error) *messageRouter {
	return &messageRouter{
		localNode: localNode,
		handler:   newMessageHandler(),
		sendFunc:  sendFunc,
	}
}

// SendToPeer sends a message to a specific peer
func (r *messageRouter) SendToPeer(nodeID string, msgType MessageType, payload interface{}) error {
	data, err := marshalMessage(msgType, r.localNode, payload)
	if err != nil {
		return err
	}

	return r.sendFunc(nodeID, data)
}

// Broadcast sends a message to all peers
func (r *messageRouter) Broadcast(msgType MessageType, payload interface{}) error {
	data, err := marshalMessage(msgType, r.localNode, payload)
	if err != nil {
		return err
	}

	// Use empty nodeID to indicate broadcast
	return r.sendFunc("", data)
}

// ProcessMessage handles an incoming message
func (r *messageRouter) ProcessMessage(data []byte) error {
	msgType, from, payload, err := unmarshalMessage(data)
	if err != nil {
		return err
	}

	return r.handler.Handle(msgType, from, payload)
}