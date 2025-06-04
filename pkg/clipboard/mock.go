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
	"sync"
	"time"
)

// MockClipboard implements a mock clipboard for testing.
// It stores clipboard content in memory and provides mechanisms
// to simulate clipboard changes for testing Watch functionality.
type MockClipboard struct {
	mu        sync.RWMutex     // Protects content
	content   string          // Current clipboard content
	watchers  []chan<- string // Active watcher channels
	watcherMu sync.Mutex      // Protects watchers slice
}

// NewMockClipboard creates a new mock clipboard instance.
// The clipboard starts with empty content and no active watchers.
func NewMockClipboard() *MockClipboard {
	return &MockClipboard{
		watchers: make([]chan<- string, 0),
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
func (m *MockClipboard) Write(content string) error {
	m.mu.Lock()
	m.content = content
	m.mu.Unlock()

	// Notify all watchers
	m.notifyWatchers(content)
	return nil
}

// Watch returns a channel that emits when the clipboard changes.
// The channel is automatically closed and the watcher is removed
// when the provided context is cancelled.
//
// Multiple watchers can be active simultaneously, and each will
// receive notifications of clipboard changes.
func (m *MockClipboard) Watch(ctx context.Context) <-chan string {
	ch := make(chan string)

	// Register the watcher
	m.watcherMu.Lock()
	m.watchers = append(m.watchers, ch)
	m.watcherMu.Unlock()

	// Clean up when context is cancelled
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
func (m *MockClipboard) EmitChange(content string) {
	m.mu.Lock()
	m.content = content
	m.mu.Unlock()

	m.notifyWatchers(content)
}

// notifyWatchers sends the new content to all registered watchers.
// Notifications are sent asynchronously with a timeout to prevent
// blocking if a watcher's channel is full or not being consumed.
func (m *MockClipboard) notifyWatchers(content string) {
	m.watcherMu.Lock()
	watchers := make([]chan<- string, len(m.watchers))
	copy(watchers, m.watchers)
	m.watcherMu.Unlock()

	// Send to all watchers in a separate goroutine to avoid blocking
	for _, ch := range watchers {
		go func(c chan<- string) {
			select {
			case c <- content:
			case <-time.After(100 * time.Millisecond):
				// Timeout to prevent hanging
			}
		}(ch)
	}
}

// GetWatcherCount returns the number of active watchers.
// This is a test helper method to verify that watchers are properly
// registered and cleaned up. It's particularly useful for testing
// that watchers are removed when their contexts are cancelled.
func (m *MockClipboard) GetWatcherCount() int {
	m.watcherMu.Lock()
	defer m.watcherMu.Unlock()
	return len(m.watchers)
}
