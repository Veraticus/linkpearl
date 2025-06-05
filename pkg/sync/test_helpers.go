package sync

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Veraticus/linkpearl/pkg/clipboard"
	"github.com/Veraticus/linkpearl/pkg/mesh"
)

// mockClipboard implements clipboard.Clipboard for testing
type mockClipboard struct {
	mu             sync.Mutex
	content        string
	changeCh       chan struct{}
	writeCh        chan string // To observe writes
	readErr        error
	writeErr       error
	watchErr       error
	sequenceNumber atomic.Uint64
	lastModified   time.Time
	contentHash    string
}

func newMockClipboard() *mockClipboard {
	return &mockClipboard{
		changeCh:     make(chan struct{}, 10),
		writeCh:      make(chan string, 10),
		lastModified: time.Now(),
		contentHash:  computeChecksum(""),
	}
}

func (m *mockClipboard) Read() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readErr != nil {
		return "", m.readErr
	}
	return m.content, nil
}

func (m *mockClipboard) Write(content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.writeErr != nil {
		return m.writeErr
	}

	m.content = content
	m.contentHash = computeChecksum(content)
	m.sequenceNumber.Add(1)
	m.lastModified = time.Now()

	select {
	case m.writeCh <- content:
	default:
	}
	return nil
}

func (m *mockClipboard) Watch(ctx context.Context) <-chan struct{} {
	if m.watchErr != nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return m.changeCh
}

func (m *mockClipboard) EmitChange(content string) {
	m.mu.Lock()
	m.content = content
	m.contentHash = computeChecksum(content)
	m.sequenceNumber.Add(1)
	m.lastModified = time.Now()
	m.mu.Unlock()

	select {
	case m.changeCh <- struct{}{}:
	default:
	}
}

func (m *mockClipboard) GetState() clipboard.ClipboardState {
	m.mu.Lock()
	defer m.mu.Unlock()

	return clipboard.ClipboardState{
		SequenceNumber: m.sequenceNumber.Load(),
		LastModified:   m.lastModified,
		ContentHash:    m.contentHash,
	}
}

// mockTopology implements mesh.Topology for testing
type mockTopology struct {
	mu         sync.Mutex
	peers      map[string]*mesh.PeerInfo
	eventCh    chan mesh.TopologyEvent
	messageCh  chan mesh.Message
	broadcasts []interface{}
	sendErr    error
}

func newMockTopology() *mockTopology {
	return &mockTopology{
		peers:     make(map[string]*mesh.PeerInfo),
		eventCh:   make(chan mesh.TopologyEvent, 10),
		messageCh: make(chan mesh.Message, 10),
	}
}

func (m *mockTopology) Start(ctx context.Context) error {
	return nil
}

func (m *mockTopology) Stop() error {
	return nil
}

func (m *mockTopology) AddJoinAddr(addr string) error {
	return nil
}

func (m *mockTopology) RemoveJoinAddr(addr string) error {
	return nil
}

func (m *mockTopology) GetPeer(nodeID string) (*mesh.PeerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if peer, exists := m.peers[nodeID]; exists {
		return peer, nil
	}
	return nil, mesh.ErrPeerNotFound
}

func (m *mockTopology) GetPeers() map[string]*mesh.PeerInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	peers := make(map[string]*mesh.PeerInfo)
	for k, v := range m.peers {
		peers[k] = v
	}
	return peers
}

func (m *mockTopology) PeerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.peers)
}

func (m *mockTopology) SendToPeer(nodeID string, msg interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}
	return nil
}

func (m *mockTopology) Broadcast(msg interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}

	m.broadcasts = append(m.broadcasts, msg)
	return nil
}

func (m *mockTopology) Events() <-chan mesh.TopologyEvent {
	return m.eventCh
}

func (m *mockTopology) Messages() <-chan mesh.Message {
	return m.messageCh
}

func (m *mockTopology) EmitMessage(from string, msgType string, payload interface{}) {
	msg := mesh.Message{
		From:    from,
		Type:    msgType,
		Payload: mustMarshal(payload),
	}

	select {
	case m.messageCh <- msg:
	default:
	}
}

func (m *mockTopology) GetBroadcasts() []interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	broadcasts := make([]interface{}, len(m.broadcasts))
	copy(broadcasts, m.broadcasts)
	return broadcasts
}

// testLogger implements Logger for testing
type testLogger struct {
	mu   sync.Mutex
	logs []logEntry
}

type logEntry struct {
	level string
	msg   string
	kv    []interface{}
}

func newTestLogger() *testLogger {
	return &testLogger{}
}

func (l *testLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.log("DEBUG", msg, keysAndValues...)
}

func (l *testLogger) Info(msg string, keysAndValues ...interface{}) {
	l.log("INFO", msg, keysAndValues...)
}

func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {
	l.log("ERROR", msg, keysAndValues...)
}

func (l *testLogger) log(level, msg string, keysAndValues ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.logs = append(l.logs, logEntry{
		level: level,
		msg:   msg,
		kv:    keysAndValues,
	})
}

func (l *testLogger) GetLogs() []logEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	logs := make([]logEntry, len(l.logs))
	copy(logs, l.logs)
	return logs
}

// Helper functions

func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
