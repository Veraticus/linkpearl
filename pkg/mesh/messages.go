// messages.go implements the message protocol and routing for the mesh topology.
// This file defines the message types, serialization format, and the routing
// infrastructure that handles incoming and outgoing messages.
//
// The mesh uses a simple JSON-based protocol where each message contains:
//   - Type: Identifies the message type for routing to appropriate handlers
//   - From: The node ID of the sender for response routing
//   - Payload: The actual message content, which varies by message type
//
// The message system supports both built-in message types (peer list, ping/pong)
// and application-defined custom message types. Message handlers can be registered
// for each message type, enabling extensible protocol implementations.

package mesh

import (
	"encoding/json"
	"fmt"
)

// MessageType identifies the type of mesh message.
// The mesh defines several built-in message types and allows applications
// to define their own custom types.
type MessageType string

const (
	// MessageTypePeerList is sent to share the list of connected peers.
	MessageTypePeerList MessageType = "peer_list"

	// MessageTypeClipboard is for clipboard sync messages.
	MessageTypeClipboard MessageType = "clipboard"

	// MessageTypePing is for keepalive/health checks.
	MessageTypePing MessageType = "ping"

	// MessageTypePong is the response to ping.
	MessageTypePong MessageType = "pong"
)

// MeshMessage is the public interface for all mesh messages.
//
//nolint:revive // Can't rename to Message due to existing Message struct
type MeshMessage interface {
	Type() MessageType
	From() string
	Marshal() ([]byte, error)
}

// ClipboardSyncMessage represents a clipboard synchronization message.
type ClipboardSyncMessage struct {
	FromNode    string
	MessageData json.RawMessage // Will contain sync.ClipboardMessage
}

// Type returns the message type.
func (m ClipboardSyncMessage) Type() MessageType { return MessageTypeClipboard }

// From returns the sender node ID.
func (m ClipboardSyncMessage) From() string { return m.FromNode }

// Marshal serializes the message for transport.
func (m ClipboardSyncMessage) Marshal() ([]byte, error) {
	wrapper := meshMessage{
		Type:    m.Type(),
		From:    m.From(),
		Payload: m.MessageData,
	}
	return json.Marshal(wrapper)
}

// PeerListSyncMessage represents a peer list exchange message.
type PeerListSyncMessage struct {
	FromNode string
	Peers    []Node
}

// Type returns the message type.
func (m PeerListSyncMessage) Type() MessageType { return MessageTypePeerList }

// From returns the sender node ID.
func (m PeerListSyncMessage) From() string { return m.FromNode }

// Marshal serializes the message for transport.
func (m PeerListSyncMessage) Marshal() ([]byte, error) {
	payload, err := json.Marshal(PeerListMessage{Peers: m.Peers})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal peer list: %w", err)
	}

	wrapper := meshMessage{
		Type:    m.Type(),
		From:    m.From(),
		Payload: payload,
	}
	return json.Marshal(wrapper)
}

// PingSyncMessage represents a ping message for keepalive.
type PingSyncMessage struct {
	FromNode  string
	Timestamp int64
}

// Type returns the message type.
func (m PingSyncMessage) Type() MessageType { return MessageTypePing }

// From returns the sender node ID.
func (m PingSyncMessage) From() string { return m.FromNode }

// Marshal serializes the message for transport.
func (m PingSyncMessage) Marshal() ([]byte, error) {
	payload, err := json.Marshal(PingMessage{Timestamp: m.Timestamp})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ping: %w", err)
	}

	wrapper := meshMessage{
		Type:    m.Type(),
		From:    m.From(),
		Payload: payload,
	}
	return json.Marshal(wrapper)
}

// PongSyncMessage represents a pong response message.
type PongSyncMessage struct {
	FromNode  string
	Timestamp int64
}

// Type returns the message type.
func (m PongSyncMessage) Type() MessageType { return MessageTypePong }

// From returns the sender node ID.
func (m PongSyncMessage) From() string { return m.FromNode }

// Marshal serializes the message for transport.
func (m PongSyncMessage) Marshal() ([]byte, error) {
	payload, err := json.Marshal(PongMessage{Timestamp: m.Timestamp})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pong: %w", err)
	}

	wrapper := meshMessage{
		Type:    m.Type(),
		From:    m.From(),
		Payload: payload,
	}
	return json.Marshal(wrapper)
}

// meshMessage is the base structure for all mesh messages.
type meshMessage struct {
	Type    MessageType     `json:"type"`
	From    string          `json:"from"`
	Payload json.RawMessage `json:"payload"`
}

// PeerListMessage contains the list of connected peers.
type PeerListMessage struct {
	Peers []Node `json:"peers"`
}

// PingMessage is sent for keepalive.
type PingMessage struct {
	Timestamp int64 `json:"timestamp"`
}

// PongMessage is the response to a ping.
type PongMessage struct {
	Timestamp int64 `json:"timestamp"`
}

// unmarshalMessage extracts the message type and payload.
func unmarshalMessage(data []byte) (MessageType, string, json.RawMessage, error) {
	var msg meshMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", "", nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return msg.Type, msg.From, msg.Payload, nil
}

// messageHandler processes incoming messages based on their type.
// It maintains a registry of handler functions mapped by message type,
// allowing different message types to be processed by specific logic.
// This enables extensible message handling where applications can register
// their own handlers for custom message types.
type messageHandler struct {
	handlers map[MessageType]func(from string, payload json.RawMessage) error
}

// newMessageHandler creates a new message handler.
func newMessageHandler() *messageHandler {
	return &messageHandler{
		handlers: make(map[MessageType]func(from string, payload json.RawMessage) error),
	}
}

// Register registers a handler for a message type.
func (h *messageHandler) Register(msgType MessageType, handler func(from string, payload json.RawMessage) error) {
	h.handlers[msgType] = handler
}

// Handle processes a message.
func (h *messageHandler) Handle(msgType MessageType, from string, payload json.RawMessage) error {
	handler, exists := h.handlers[msgType]
	if !exists {
		return fmt.Errorf("no handler for message type: %s", msgType)
	}

	return handler(from, payload)
}

// NewClipboardMessage creates a new clipboard sync message.
func NewClipboardMessage(from string, data json.RawMessage) ClipboardSyncMessage {
	return ClipboardSyncMessage{FromNode: from, MessageData: data}
}

// NewPeerListMessage creates a new peer list message.
func NewPeerListMessage(from string, peers []Node) PeerListSyncMessage {
	return PeerListSyncMessage{FromNode: from, Peers: peers}
}

// NewPingMessage creates a new ping message.
func NewPingMessage(from string, timestamp int64) PingSyncMessage {
	return PingSyncMessage{FromNode: from, Timestamp: timestamp}
}

// NewPongMessage creates a new pong message.
func NewPongMessage(from string, timestamp int64) PongSyncMessage {
	return PongSyncMessage{FromNode: from, Timestamp: timestamp}
}

// messageRouter routes messages between peers and local handlers.
// It acts as the central message processing component, handling both
// incoming messages (routing them to appropriate handlers) and outgoing
// messages (serializing and sending them to peers).
//
// The router supports both unicast (peer-to-peer) and broadcast messaging
// patterns, automatically handling message serialization and adding the
// sender information to outgoing messages.
type messageRouter struct {
	handler   *messageHandler
	sendFunc  func(nodeID string, data []byte) error
	localNode string
}

// newMessageRouter creates a new message router.
func newMessageRouter(localNode string, sendFunc func(nodeID string, data []byte) error) *messageRouter {
	return &messageRouter{
		localNode: localNode,
		handler:   newMessageHandler(),
		sendFunc:  sendFunc,
	}
}

// SendToPeer sends a message to a specific peer.
func (r *messageRouter) SendToPeer(nodeID string, msgType MessageType, payload any) error {
	// Create appropriate message type
	var msg MeshMessage
	switch msgType {
	case MessageTypePing:
		if ping, ok := payload.(PingMessage); ok {
			msg = NewPingMessage(r.localNode, ping.Timestamp)
		} else {
			return fmt.Errorf("invalid payload for ping message")
		}
	case MessageTypePong:
		if pong, ok := payload.(PongMessage); ok {
			msg = NewPongMessage(r.localNode, pong.Timestamp)
		} else {
			return fmt.Errorf("invalid payload for pong message")
		}
	case MessageTypePeerList:
		if peerList, ok := payload.(PeerListMessage); ok {
			msg = NewPeerListMessage(r.localNode, peerList.Peers)
		} else {
			return fmt.Errorf("invalid payload for peer list message")
		}
	default:
		return fmt.Errorf("unsupported message type: %s", msgType)
	}

	// Marshal the type-safe message
	data, err := msg.Marshal()
	if err != nil {
		return err
	}

	return r.sendFunc(nodeID, data)
}

// Broadcast sends a message to all peers.
func (r *messageRouter) Broadcast(msgType MessageType, payload any) error {
	// Create appropriate message type
	var msg MeshMessage
	switch msgType {
	case MessageTypePing:
		if ping, ok := payload.(PingMessage); ok {
			msg = NewPingMessage(r.localNode, ping.Timestamp)
		} else {
			return fmt.Errorf("invalid payload for ping message")
		}
	case MessageTypePong:
		if pong, ok := payload.(PongMessage); ok {
			msg = NewPongMessage(r.localNode, pong.Timestamp)
		} else {
			return fmt.Errorf("invalid payload for pong message")
		}
	case MessageTypePeerList:
		if peerList, ok := payload.(PeerListMessage); ok {
			msg = NewPeerListMessage(r.localNode, peerList.Peers)
		} else {
			return fmt.Errorf("invalid payload for peer list message")
		}
	default:
		return fmt.Errorf("unsupported message type: %s", msgType)
	}

	// Marshal the type-safe message
	data, err := msg.Marshal()
	if err != nil {
		return err
	}

	// Use empty nodeID to indicate broadcast
	return r.sendFunc("", data)
}

// ProcessMessage handles an incoming message.
func (r *messageRouter) ProcessMessage(data []byte) error {
	msgType, from, payload, err := unmarshalMessage(data)
	if err != nil {
		return err
	}

	return r.handler.Handle(msgType, from, payload)
}
