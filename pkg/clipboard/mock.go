// This file provides a mock implementation of the Clipboard interface for testing.
//
// The MockClipboard allows tests to simulate clipboard operations without
// accessing the actual system clipboard. This is essential for:
//   - Unit testing clipboard-dependent code
//   - Running tests in environments without clipboard access (e.g., CI/CD)
//   - Simulating specific clipboard behaviors and edge cases
//   - Testing concurrent access patterns
//
// Key Features:
//   - In-memory clipboard storage
//   - Synchronous read/write operations
//   - Simulated change notifications for Watch
//   - Helper methods for test scenarios
//   - Thread-safe implementation
//
// Usage in Tests:
//
//	mock := NewMockClipboard()
//
//	// Test write operation
//	err := mock.Write("test content")
//	assert.NoError(t, err)
//
//	// Test read operation
//	content, err := mock.Read()
//	assert.NoError(t, err)
//	assert.Equal(t, "test content", content)
//
//	// Test watch functionality
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	changes := mock.Watch(ctx)
//
//	// Simulate clipboard change
//	mock.EmitChange("new content")
//
//	select {
//	case content := <-changes:
//	    assert.Equal(t, "new content", content)
//	case <-time.After(time.Second):
//	    t.Fatal("timeout waiting for change")
//	}
//
// Advanced Testing:
//
// The mock provides additional methods for testing complex scenarios:
//   - EmitChange: Simulate external clipboard changes
//   - GetWatcherCount: Verify watcher cleanup
//
// These methods enable testing of edge cases like:
//   - Multiple concurrent watchers
//   - Watcher lifecycle management
//   - Rapid clipboard changes
//   - Context cancellation behavior

package clipboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"
)

// MockClipboard implements a mock clipboard for testing.
// It stores clipboard content in memory and provides mechanisms
// to simulate clipboard changes for testing Watch functionality.
type MockClipboard struct {
	lastModified   time.Time
	content        string
	contentHash    string
	watchers       []chan<- struct{}
	sequenceNumber atomic.Uint64
	mu             sync.RWMutex
	watcherMu      sync.Mutex
}

// NewMockClipboard creates a new mock clipboard instance.
// The clipboard starts with empty content and no active watchers.
func NewMockClipboard() *MockClipboard {
	return &MockClipboard{
		watchers:     make([]chan<- struct{}, 0),
		lastModified: time.Now(),
		contentHash:  hashContent(""),
	}
}

// Read returns the current mock clipboard contents.
// This operation never fails in the mock implementation.
func (m *MockClipboard) Read() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.content, nil
}

// Write sets the mock clipboard contents and notifies watchers.
// This simulates a clipboard write operation and triggers all
// active watchers to receive the new content.
// Content size is limited to MaxClipboardSize to match production behavior.
func (m *MockClipboard) Write(content string) error {
	// Validate content (same as production)
	if err := ValidateContent([]byte(content)); err != nil {
		return err
	}

	m.mu.Lock()
	m.content = content
	m.contentHash = hashContent(content)
	m.sequenceNumber.Add(1)
	m.lastModified = time.Now()
	m.mu.Unlock()

	// Notify all watchers
	m.notifyWatchers()
	return nil
}

// Watch returns a channel that signals when clipboard state changes.
// The channel receives empty structs as notifications.
// Actual content must be retrieved using Read().
// Channel has buffer size of 10 to handle bursts without blocking.
//
// The channel is automatically closed and the watcher is removed
// when the provided context is canceled.
//
// Multiple watchers can be active simultaneously, and each will
// receive notifications of clipboard changes.
func (m *MockClipboard) Watch(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, 10)

	// Register the watcher
	m.watcherMu.Lock()
	m.watchers = append(m.watchers, ch)
	m.watcherMu.Unlock()

	// Clean up when context is canceled
	go func() {
		<-ctx.Done()
		m.watcherMu.Lock()
		defer m.watcherMu.Unlock()

		// Remove this watcher
		for i, watcher := range m.watchers {
			if watcher == ch {
				m.watchers = append(m.watchers[:i], m.watchers[i+1:]...)
				break
			}
		}
		close(ch)
	}()

	return ch
}

// EmitChange simulates a clipboard change for testing.
// This method allows tests to simulate external clipboard changes
// without going through the Write method. It updates the content
// and notifies all watchers.
//
// This is useful for testing scenarios where clipboard content
// changes from outside the application being tested.
// Note: This method bypasses size limit checks for testing purposes.
func (m *MockClipboard) EmitChange(content string) {
	m.mu.Lock()
	m.content = content
	m.contentHash = hashContent(content)
	m.sequenceNumber.Add(1)
	m.lastModified = time.Now()
	m.mu.Unlock()

	m.notifyWatchers()
}

// notifyWatchers sends notifications to all registered watchers.
// Notifications are sent as empty structs to signal state change.
// Actual content must be retrieved using Read().
func (m *MockClipboard) notifyWatchers() {
	m.watcherMu.Lock()
	watchers := make([]chan<- struct{}, len(m.watchers))
	copy(watchers, m.watchers)
	m.watcherMu.Unlock()

	// Send to all watchers (non-blocking)
	for _, ch := range watchers {
		select {
		case ch <- struct{}{}:
			// Successfully sent
		default:
			// Channel full - skip notification
		}
	}
}

// GetWatcherCount returns the number of active watchers.
// This is a test helper method to verify that watchers are properly
// registered and cleaned up. It's particularly useful for testing
// that watchers are removed when their contexts are canceled.
func (m *MockClipboard) GetWatcherCount() int {
	m.watcherMu.Lock()
	defer m.watcherMu.Unlock()
	return len(m.watchers)
}

// GetState returns current state information.
func (m *MockClipboard) GetState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return State{
		SequenceNumber: m.sequenceNumber.Load(),
		LastModified:   m.lastModified,
		ContentHash:    m.contentHash,
	}
}

// hashContent creates a SHA-256 hash of the content.
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
