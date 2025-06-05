package clipboard

import (
	"context"
	"sync"
	"time"
)

// NoopClipboard is a clipboard implementation that doesn't interact with
// any actual clipboard. It's useful for headless servers or when using
// linkpearl as a network pipe without clipboard integration.
//
//nolint:govet // fieldalignment: not performance critical
type NoopClipboard struct {
	mu            sync.RWMutex
	watchMu       sync.Mutex
	state         State
	content       string
	watchChannels []chan struct{}
}

// NewNoopClipboard creates a new no-op clipboard implementation.
func NewNoopClipboard() *NoopClipboard {
	return &NoopClipboard{
		content: "",
		state: State{
			SequenceNumber: 0,
			LastModified:   time.Now(),
			ContentHash:    hashContent(""),
		},
	}
}

// Read returns the current clipboard content.
func (c *NoopClipboard) Read() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.content, nil
}

// Write sets the clipboard content.
func (c *NoopClipboard) Write(content string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.content = content
	c.state.SequenceNumber++
	c.state.LastModified = time.Now()
	c.state.ContentHash = hashContent(content)

	// Notify watchers
	c.watchMu.Lock()
	for _, ch := range c.watchChannels {
		select {
		case ch <- struct{}{}:
		default:
			// Channel full, skip
		}
	}
	c.watchMu.Unlock()

	return nil
}

// Watch returns a channel that receives notifications when content changes.
func (c *NoopClipboard) Watch(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, 10)

	c.watchMu.Lock()
	c.watchChannels = append(c.watchChannels, ch)
	c.watchMu.Unlock()

	go func() {
		<-ctx.Done()
		c.watchMu.Lock()
		// Remove channel from list
		for i, wch := range c.watchChannels {
			if wch == ch {
				c.watchChannels = append(c.watchChannels[:i], c.watchChannels[i+1:]...)
				break
			}
		}
		c.watchMu.Unlock()
		close(ch)
	}()

	return ch
}

// GetState returns the current clipboard state.
func (c *NoopClipboard) GetState() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}
