package sync

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Veraticus/linkpearl/pkg/mesh"
)

func TestNewEngine(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := &Config{
			NodeID:    "test-node",
			Clipboard: newMockClipboard(),
			Topology:  newMockTopology(),
		}
		
		engine, err := NewEngine(config)
		require.NoError(t, err)
		require.NotNil(t, engine)
		
		// Check defaults were applied
		assert.Equal(t, 1000, config.DedupeSize)
		assert.Equal(t, 500*time.Millisecond, config.SyncLoopWindow)
		assert.Equal(t, 100*time.Millisecond, config.MinChangeInterval)
		assert.NotNil(t, config.Logger)
	})
	
	t.Run("invalid config", func(t *testing.T) {
		tests := []struct {
			name   string
			config *Config
			errMsg string
		}{
			{
				name:   "missing node ID",
				config: &Config{Clipboard: newMockClipboard(), Topology: newMockTopology()},
				errMsg: "node ID is required",
			},
			{
				name:   "missing clipboard",
				config: &Config{NodeID: "test", Topology: newMockTopology()},
				errMsg: "clipboard is required",
			},
			{
				name:   "missing topology",
				config: &Config{NodeID: "test", Clipboard: newMockClipboard()},
				errMsg: "topology is required",
			},
		}
		
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				engine, err := NewEngine(tt.config)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, engine)
			})
		}
	})
}

func TestEngineLocalChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	clipboard := newMockClipboard()
	topology := newMockTopology()
	logger := newTestLogger()
	
	config := &Config{
		NodeID:    "test-node",
		Clipboard: clipboard,
		Topology:  topology,
		Logger:    logger,
	}
	
	engine, err := NewEngine(config)
	require.NoError(t, err)
	
	// Start engine in background
	go func() { _ = engine.Run(ctx) }()
	
	// Give engine time to initialize
	time.Sleep(50 * time.Millisecond)
	
	// Simulate local clipboard change
	clipboard.EmitChange("Hello, World!")
	
	// Wait for broadcast
	assert.Eventually(t, func() bool {
		broadcasts := topology.GetBroadcasts()
		return len(broadcasts) == 1
	}, time.Second, 10*time.Millisecond)
	
	// Verify broadcast message
	broadcasts := topology.GetBroadcasts()
	require.Len(t, broadcasts, 1)
	
	msgMap, ok := broadcasts[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, MessageTypeClipboard, msgMap["type"])
	
	// Parse the payload
	payloadBytes := msgMap["payload"].(json.RawMessage)
	var clipMsg ClipboardMessage
	err = json.Unmarshal(payloadBytes, &clipMsg)
	require.NoError(t, err)
	
	assert.Equal(t, "test-node", clipMsg.NodeID)
	assert.Equal(t, "Hello, World!", clipMsg.Content)
	assert.Equal(t, computeChecksum("Hello, World!"), clipMsg.Checksum)
	assert.Greater(t, clipMsg.Timestamp, int64(0))
	
	// Check stats
	stats := engine.Stats()
	assert.Equal(t, uint64(1), stats.LocalChanges)
	assert.Equal(t, uint64(1), stats.MessagesSent)
}

func TestEngineRemoteChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	clipboard := newMockClipboard()
	topology := newMockTopology()
	
	config := &Config{
		NodeID:    "test-node",
		Clipboard: clipboard,
		Topology:  topology,
	}
	
	engine, err := NewEngine(config)
	require.NoError(t, err)
	
	// Start engine
	go func() { _ = engine.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	
	// Create remote message
	remoteMsg := NewClipboardMessage("remote-node", "Remote content")
	topology.EmitMessage("remote-node", string(MessageTypeClipboard), remoteMsg)
	
	// Wait for clipboard update
	assert.Eventually(t, func() bool {
		content, _ := clipboard.Read()
		return content == "Remote content"
	}, time.Second, 10*time.Millisecond)
	
	// Verify clipboard was updated
	content, err := clipboard.Read()
	require.NoError(t, err)
	assert.Equal(t, "Remote content", content)
	
	// Check stats
	stats := engine.Stats()
	assert.Equal(t, uint64(1), stats.MessagesReceived)
	assert.Equal(t, uint64(1), stats.RemoteChanges)
}

func TestEngineConflictResolution(t *testing.T) {
	t.Run("remote wins with newer timestamp", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		
		clipboard := newMockClipboard()
		clipboard.content = "Local content"
		topology := newMockTopology()
		
		config := &Config{
			NodeID:    "node-a",
			Clipboard: clipboard,
			Topology:  topology,
		}
		
		engine, err := NewEngine(config)
		require.NoError(t, err)
		
		go func() { _ = engine.Run(ctx) }()
		time.Sleep(50 * time.Millisecond)
		
		// Remote message with newer timestamp
		remoteMsg := &ClipboardMessage{
			NodeID:    "node-b",
			Timestamp: time.Now().Add(time.Hour).UnixNano(), // Future timestamp
			Content:   "Remote wins",
			Checksum:  computeChecksum("Remote wins"),
			Version:   "1.0",
		}
		
		topology.EmitMessage("node-b", string(MessageTypeClipboard), remoteMsg)
		
		// Wait for update
		assert.Eventually(t, func() bool {
			content, _ := clipboard.Read()
			return content == "Remote wins"
		}, time.Second, 10*time.Millisecond)
		
		stats := engine.Stats()
		assert.Equal(t, uint64(1), stats.ConflictsLost)
	})
	
	t.Run("local wins with newer timestamp", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		
		clipboard := newMockClipboard()
		topology := newMockTopology()
		
		config := &Config{
			NodeID:    "node-a",
			Clipboard: clipboard,
			Topology:  topology,
		}
		
		engine, err := NewEngine(config)
		require.NoError(t, err)
		
		go func() { _ = engine.Run(ctx) }()
		time.Sleep(50 * time.Millisecond)
		
		// Set current state
		clipboard.EmitChange("Current content")
		time.Sleep(100 * time.Millisecond)
		
		// Remote message with older timestamp
		remoteMsg := &ClipboardMessage{
			NodeID:    "node-b",
			Timestamp: time.Now().Add(-time.Hour).UnixNano(), // Past timestamp
			Content:   "Old content",
			Checksum:  computeChecksum("Old content"),
			Version:   "1.0",
		}
		
		topology.EmitMessage("node-b", string(MessageTypeClipboard), remoteMsg)
		time.Sleep(100 * time.Millisecond)
		
		// Content should not change
		content, _ := clipboard.Read()
		assert.Equal(t, "Current content", content)
		
		stats := engine.Stats()
		assert.Equal(t, uint64(0), stats.RemoteChanges)
	})
}

func TestEngineDeduplication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	clipboard := newMockClipboard()
	topology := newMockTopology()
	
	config := &Config{
		NodeID:    "test-node",
		Clipboard: clipboard,
		Topology:  topology,
	}
	
	engine, err := NewEngine(config)
	require.NoError(t, err)
	
	go func() { _ = engine.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	
	// Send same message multiple times
	msg := NewClipboardMessage("remote-node", "Test content")
	
	for i := 0; i < 5; i++ {
		topology.EmitMessage("remote-node", string(MessageTypeClipboard), msg)
		time.Sleep(10 * time.Millisecond)
	}
	
	time.Sleep(100 * time.Millisecond)
	
	// Should only apply once
	stats := engine.Stats()
	assert.Equal(t, uint64(5), stats.MessagesReceived)
	assert.Equal(t, uint64(1), stats.RemoteChanges)
	assert.Equal(t, uint64(4), stats.MessagesDuplicate)
}

func TestEngineSyncLoopDetection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	clipboard := newMockClipboard()
	topology := newMockTopology()
	
	config := &Config{
		NodeID:            "test-node",
		Clipboard:         clipboard,
		Topology:          topology,
		MinChangeInterval: 200 * time.Millisecond,
	}
	
	engine, err := NewEngine(config)
	require.NoError(t, err)
	
	go func() { _ = engine.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	
	// Simulate remote change
	remoteMsg := NewClipboardMessage("remote-node", "Remote content")
	topology.EmitMessage("remote-node", string(MessageTypeClipboard), remoteMsg)
	
	// Wait for clipboard update
	assert.Eventually(t, func() bool {
		content, _ := clipboard.Read()
		return content == "Remote content"
	}, time.Second, 10*time.Millisecond)
	
	// Immediately simulate local change (potential sync loop)
	clipboard.EmitChange("Remote content modified")
	
	time.Sleep(300 * time.Millisecond)
	
	// Check that we didn't broadcast (sync loop detected)
	broadcasts := topology.GetBroadcasts()
	assert.Len(t, broadcasts, 0)
}

func TestEngineTopologyEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	clipboard := newMockClipboard()
	topology := newMockTopology()
	logger := newTestLogger()
	
	config := &Config{
		NodeID:    "test-node",
		Clipboard: clipboard,
		Topology:  topology,
		Logger:    logger,
	}
	
	engine, err := NewEngine(config)
	require.NoError(t, err)
	
	go func() { _ = engine.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	
	// Emit peer connected event
	event := mesh.TopologyEvent{
		Type: mesh.PeerConnected,
		Peer: mesh.Node{ID: "peer-1", Mode: "full"},
		Time: time.Now(),
	}
	
	select {
	case topology.eventCh <- event:
	case <-time.After(time.Second):
		t.Fatal("failed to send event")
	}
	
	time.Sleep(50 * time.Millisecond)
	
	// Check logs
	logs := logger.GetLogs()
	found := false
	for _, log := range logs {
		if log.msg == "peer connected" {
			found = true
			break
		}
	}
	assert.True(t, found, "peer connected log not found")
}

func TestEngineErrorHandling(t *testing.T) {
	t.Run("clipboard read error on init", func(t *testing.T) {
		clipboard := newMockClipboard()
		clipboard.readErr = assert.AnError
		topology := newMockTopology()
		
		config := &Config{
			NodeID:    "test-node",
			Clipboard: clipboard,
			Topology:  topology,
		}
		
		// Should not fail - starts with empty clipboard
		engine, err := NewEngine(config)
		require.NoError(t, err)
		require.NotNil(t, engine)
	})
	
	t.Run("broadcast error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		
		clipboard := newMockClipboard()
		topology := newMockTopology()
		topology.sendErr = assert.AnError
		
		config := &Config{
			NodeID:    "test-node",
			Clipboard: clipboard,
			Topology:  topology,
		}
		
		engine, err := NewEngine(config)
		require.NoError(t, err)
		
		go func() { _ = engine.Run(ctx) }()
		time.Sleep(50 * time.Millisecond)
		
		// Trigger local change
		clipboard.EmitChange("Test")
		time.Sleep(100 * time.Millisecond)
		
		// Check error was counted
		stats := engine.Stats()
		assert.Equal(t, uint64(1), stats.SendErrors)
	})
	
	t.Run("invalid message", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		
		clipboard := newMockClipboard()
		topology := newMockTopology()
		
		config := &Config{
			NodeID:    "test-node",
			Clipboard: clipboard,
			Topology:  topology,
		}
		
		engine, err := NewEngine(config)
		require.NoError(t, err)
		
		go func() { _ = engine.Run(ctx) }()
		time.Sleep(50 * time.Millisecond)
		
		// Send invalid message
		topology.EmitMessage("bad-node", string(MessageTypeClipboard), "not-a-valid-message")
		time.Sleep(100 * time.Millisecond)
		
		stats := engine.Stats()
		assert.Equal(t, uint64(1), stats.ReceiveErrors)
	})
}

func TestEngineStats(t *testing.T) {
	clipboard := newMockClipboard()
	topology := newMockTopology()
	
	config := &Config{
		NodeID:    "test-node",
		Clipboard: clipboard,
		Topology:  topology,
	}
	
	engine, err := NewEngine(config)
	require.NoError(t, err)
	
	stats := engine.Stats()
	assert.NotNil(t, stats)
	assert.False(t, stats.StartTime.IsZero())
	
	// Stats should be a copy
	stats.MessagesSent = 999
	stats2 := engine.Stats()
	assert.Equal(t, uint64(0), stats2.MessagesSent)
}