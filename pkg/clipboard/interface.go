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
//   - State-based clipboard monitoring with Watch
//   - Efficient change detection to avoid duplicate notifications
//   - Mock implementation for testing
//   - Non-blocking event notification
//   - Bounded memory usage
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
//	// Watch for clipboard changes (state-based)
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	notifications := clip.Watch(ctx)
//	for range notifications {
//	    // Notification received - pull current state
//	    content, err := clip.Read()
//	    if err != nil {
//	        log.Printf("failed to read clipboard: %v", err)
//	        continue
//	    }
//	    fmt.Println("Clipboard changed to:", content)
//	}
//
// State-Based Design:
//
// The clipboard uses a state-based synchronization pattern where Watch() returns
// notifications that the clipboard state has changed, rather than the actual content.
// This design ensures:
//   - Non-blocking: Watchers never block on clipboard data
//   - Memory-bounded: No queuing of clipboard contents
//   - Pull-based: Consumers read when ready, not when pushed
//   - Natural deduplication: Multiple rapid changes may result in a single notification
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
// The Watch method returns a channel that emits empty structs when changes
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
	"time"
)

// ErrNotSupported indicates the platform is not supported.
var ErrNotSupported = errors.New("clipboard: platform not supported")

// ErrContentTooLarge indicates the content exceeds the maximum allowed size.
var ErrContentTooLarge = errors.New("clipboard: content exceeds maximum size")

// MaxClipboardSize is the maximum allowed clipboard content size (10MB).
const MaxClipboardSize = 10 * 1024 * 1024 // 10MB

// Clipboard defines the interface for platform-specific clipboard access.
type Clipboard interface {
	// Read returns the current clipboard contents
	Read() (string, error)

	// Write sets the clipboard contents
	Write(content string) error

	// Watch returns a channel that signals when clipboard state changes.
	// The channel receives empty structs as notifications.
	// Actual content must be retrieved using Read().
	// Channel has buffer size of 10 to handle bursts without blocking.
	// Multiple rapid changes may result in a single notification.
	// The channel is closed when the context is cancelled.
	Watch(ctx context.Context) <-chan struct{}

	// GetState returns current state information
	GetState() ClipboardState
}

// ClipboardState provides metadata about clipboard state.
type ClipboardState struct {
	SequenceNumber uint64    // Monotonically increasing counter
	LastModified   time.Time // When clipboard was last changed
	ContentHash    string    // SHA256 hash of current content
}

// NewPlatformClipboard returns a clipboard implementation for the current platform.
func NewPlatformClipboard() (Clipboard, error) {
	return newPlatformClipboard()
}

// ClipboardOptions configures the hardened clipboard.
type ClipboardOptions struct {
	// Enable retry logic with exponential backoff
	EnableRetry bool

	// Custom retry configuration (nil uses defaults)
	RetryConfig *RetryConfig

	// Enable rate limiting
	EnableRateLimit bool

	// Rate limit configuration (operations per minute)
	RateLimitOpsPerMinute int

	// Enable metrics collection
	EnableMetrics bool

	// Custom metrics collector (nil uses default in-memory collector)
	MetricsCollector MetricsCollector
}

// DefaultClipboardOptions returns sensible default options.
func DefaultClipboardOptions() *ClipboardOptions {
	return &ClipboardOptions{
		EnableRetry:           true,
		EnableRateLimit:       true,
		RateLimitOpsPerMinute: 100,
		EnableMetrics:         true,
	}
}

// NewHardenedClipboard creates a production-ready clipboard with all hardening features.
func NewHardenedClipboard(options *ClipboardOptions) (Clipboard, error) {
	if options == nil {
		options = DefaultClipboardOptions()
	}

	// Create base platform clipboard
	base, err := NewPlatformClipboard()
	if err != nil {
		return nil, err
	}

	// Layer on hardening features
	clipboard := base

	// Add resilience layer (retry + rate limiting + fallback)
	if options.EnableRetry || options.EnableRateLimit {
		resilient := NewResilientClipboard(clipboard)

		if options.RetryConfig != nil {
			resilient.SetRetryConfig(options.RetryConfig)
		}

		if options.EnableRateLimit && options.RateLimitOpsPerMinute > 0 {
			limiter := NewRateLimiter(options.RateLimitOpsPerMinute, time.Minute)
			resilient.SetRateLimiter(limiter)
		}

		clipboard = resilient
	}

	// Add metrics layer
	if options.EnableMetrics {
		collector := options.MetricsCollector
		if collector == nil {
			collector = NewDefaultMetricsCollector()
		}
		clipboard = NewInstrumentedClipboard(clipboard, collector)
	}

	return clipboard, nil
}
