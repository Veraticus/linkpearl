// Package clipboard provides a cross-platform interface for accessing the system clipboard.
//
// This package abstracts platform-specific clipboard operations behind a common interface,
// supporting macOS, Linux, and providing a mock implementation for testing. The package
// handles the complexities of different clipboard mechanisms across platforms while
// providing a simple, consistent API.
//
// Key Features:
//   - Cross-platform support (macOS, Linux)
//   - Synchronous read/write operations
//   - Asynchronous clipboard monitoring with Watch
//   - Efficient change detection to avoid duplicate notifications
//   - Mock implementation for testing
//
// Basic Usage:
//
//	// Create a platform-specific clipboard instance
//	clip, err := clipboard.NewPlatformClipboard()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Write to clipboard
//	err = clip.Write("Hello, clipboard!")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Read from clipboard
//	content, err := clip.Read()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Clipboard contains:", content)
//
//	// Watch for clipboard changes
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	changes := clip.Watch(ctx)
//	for content := range changes {
//	    fmt.Println("Clipboard changed to:", content)
//	}
//
// Platform Support:
//
// macOS:
//   - Uses pbcopy/pbpaste commands for read/write operations
//   - Leverages NSPasteboard's changeCount for efficient change detection
//   - Implements smart polling with adaptive intervals
//
// Linux:
//   - Supports multiple clipboard tools: wl-clipboard (Wayland), xsel, xclip
//   - Automatically detects and uses the first available tool
//   - Uses clipnotify for efficient monitoring when available
//   - Falls back to smart polling if clipnotify is not installed
//
// Unsupported Platforms:
//   - Returns ErrNotSupported when attempting to create a clipboard instance
//
// Implementation Notes:
//
// The package uses content hashing (SHA-256) to detect actual content changes,
// preventing duplicate notifications when the clipboard is updated with the same
// content. This is particularly important for polling-based implementations.
//
// The Watch method returns a channel that emits new clipboard content when changes
// are detected. The implementation uses different strategies based on platform
// capabilities:
//   - Event-based monitoring where available (e.g., clipnotify on Linux)
//   - Smart polling with adaptive intervals that slow down during idle periods
//   - Platform-specific optimizations (e.g., changeCount on macOS)
//
// Thread Safety:
//
// All implementations are thread-safe and can be safely used from multiple
// goroutines concurrently.
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
