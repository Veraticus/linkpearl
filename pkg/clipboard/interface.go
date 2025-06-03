package clipboard

import (
	"context"
	"errors"
)

// ErrNotSupported indicates the platform is not supported
var ErrNotSupported = errors.New("clipboard: platform not supported")

// Clipboard defines the interface for platform-specific clipboard access
type Clipboard interface {
	// Read returns the current clipboard contents
	Read() (string, error)

	// Write sets the clipboard contents
	Write(content string) error

	// Watch returns a channel that emits clipboard content when it changes
	// The channel is closed when the context is cancelled
	// Implementations may use different strategies:
	// - Event-based monitoring where available
	// - Smart polling with change detection
	Watch(ctx context.Context) <-chan string
}

// NewPlatformClipboard returns a clipboard implementation for the current platform
func NewPlatformClipboard() (Clipboard, error) {
	return newPlatformClipboard()
}