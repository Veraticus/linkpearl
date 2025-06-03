package clipboard

import (
	"context"
	"sync"
	"time"
)

// MockClipboard implements a mock clipboard for testing
type MockClipboard struct {
	mu        sync.RWMutex
	content   string
	watchers  []chan<- string
	watcherMu sync.Mutex
}

// NewMockClipboard creates a new mock clipboard
func NewMockClipboard() *MockClipboard {
	return &MockClipboard{
		watchers: make([]chan<- string, 0),
	}
}

// Read returns the current mock clipboard contents
func (m *MockClipboard) Read() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.content, nil
}

// Write sets the mock clipboard contents and notifies watchers
func (m *MockClipboard) Write(content string) error {
	m.mu.Lock()
	m.content = content
	m.mu.Unlock()

	// Notify all watchers
	m.notifyWatchers(content)
	return nil
}

// Watch returns a channel that emits when the clipboard changes
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

// EmitChange simulates a clipboard change for testing
func (m *MockClipboard) EmitChange(content string) {
	m.mu.Lock()
	m.content = content
	m.mu.Unlock()

	m.notifyWatchers(content)
}

// notifyWatchers sends the new content to all registered watchers
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

// GetWatcherCount returns the number of active watchers (for testing)
func (m *MockClipboard) GetWatcherCount() int {
	m.watcherMu.Lock()
	defer m.watcherMu.Unlock()
	return len(m.watchers)
}