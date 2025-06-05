//go:build darwin
// +build darwin

// This file implements clipboard access for macOS using the pbcopy and pbpaste commands.
//
// Implementation Details:
//
// The macOS implementation leverages the system's built-in pbcopy and pbpaste commands
// for clipboard operations. These commands interface with NSPasteboard, the macOS
// clipboard service.
//
// Key Features:
//   - Uses pbpaste to read clipboard contents
//   - Uses pbcopy to write clipboard contents
//   - Monitors changes using NSPasteboard's changeCount property
//   - Implements smart polling with adaptive intervals
//
// Change Detection:
//
// macOS provides a changeCount property on NSPasteboard that increments whenever
// the clipboard content changes. This implementation uses AppleScript to access
// this property, providing more efficient change detection than content polling alone.
//
// The Watch method combines changeCount monitoring with content hashing to ensure:
//   - Fast detection of clipboard changes (via changeCount)
//   - Accurate change validation (via content hashing)
//   - No duplicate notifications for the same content
//
// Performance Optimizations:
//   - Adaptive polling intervals: starts at 500ms, slows to 2s when idle
//   - ChangeCount check is fast and doesn't require reading full content
//   - Content is only read when changeCount indicates a change
//   - SHA-256 hashing prevents duplicate notifications
//
// Thread Safety:
//
// The implementation uses sync.RWMutex to protect shared state (lastHash and
// lastChangeCount), ensuring thread-safe access from multiple goroutines.

package clipboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DarwinClipboard implements clipboard access on macOS using pbcopy/pbpaste.
// It maintains internal state for efficient change detection using both
// content hashing and NSPasteboard's changeCount.
type DarwinClipboard struct {
	mu              sync.RWMutex
	lastHash        string // SHA-256 hash of last known clipboard content
	lastChangeCount int    // NSPasteboard changeCount value
	sequenceNumber  atomic.Uint64
	lastModified    time.Time
	cmdConfig       *CommandConfig // Configuration for command execution
}

// newPlatformClipboard returns a macOS clipboard implementation.
// On macOS, clipboard access is always available through pbcopy/pbpaste,
// so this function never returns an error.
func newPlatformClipboard() (Clipboard, error) {
	return &DarwinClipboard{
		cmdConfig: DefaultCommandConfig(),
	}, nil
}

// Read returns the current clipboard contents using the pbpaste command.
// This is a synchronous operation that executes pbpaste and returns its output.
// The operation has a 5-second timeout to prevent hanging on system issues.
// Content size is checked to prevent memory issues with extremely large clipboard data.
func (c *DarwinClipboard) Read() (string, error) {
	output, err := RunCommand("pbpaste", nil, c.cmdConfig)
	if err != nil {
		return "", fmt.Errorf("clipboard read failed: %w", err)
	}

	if err := ValidateContent(output); err != nil {
		return "", err
	}

	if os.Getenv("CI") == "true" {
		fmt.Printf("[DarwinClipboard.Read] Read %d bytes from clipboard\n", len(output))
	}

	return string(output), nil
}

// Write sets the clipboard contents using the pbcopy command.
// The content is piped to pbcopy's stdin. After a successful write,
// the internal tracking state is updated to reflect the new content.
// The operation has a 5-second timeout to prevent hanging on system issues.
// Content size is limited to MaxClipboardSize to prevent memory issues.
func (c *DarwinClipboard) Write(content string) error {
	contentBytes := []byte(content)
	if err := ValidateContent(contentBytes); err != nil {
		return err
	}

	if err := RunCommandWithInput("pbcopy", nil, contentBytes, c.cmdConfig); err != nil {
		return fmt.Errorf("clipboard write failed: %w", err)
	}

	// Update our tracking state only after successful write
	c.mu.Lock()
	c.lastHash = c.hashContent(content)
	c.lastChangeCount = c.getChangeCount()
	c.sequenceNumber.Add(1)
	c.lastModified = time.Now()
	c.mu.Unlock()

	return nil
}

// Watch monitors the clipboard for changes using macOS changeCount.
// It returns a channel that signals when clipboard state changes.
// The channel receives empty structs as notifications.
// Actual content must be retrieved using Read().
//
// The polling interval adapts based on activity:
//   - Active: 500ms (when changes are detected)
//   - Idle: 2s (after 10 consecutive polls with no changes)
//
// The channel is closed when the context is cancelled.
func (c *DarwinClipboard) Watch(ctx context.Context) <-chan struct{} {
	// Use buffered channel to prevent blocking the watcher goroutine
	// Buffer size of 10 allows for burst of changes without blocking
	ch := make(chan struct{}, 10)

	go func() {
		defer close(ch)

		// Initialize with current state
		if content, err := c.Read(); err == nil {
			c.mu.Lock()
			c.lastHash = c.hashContent(content)
			c.lastChangeCount = c.getChangeCount()
			c.mu.Unlock()
		}

		// Start with faster polling, slow down when idle
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		idleCount := 0
		const maxIdleCount = 10

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Check changeCount first (fast)
				currentCount := c.getChangeCount()

				c.mu.RLock()
				lastCount := c.lastChangeCount
				c.mu.RUnlock()

				if currentCount != lastCount && currentCount >= 0 {
					// Change detected, read content
					content, err := c.Read()
					if err != nil {
						continue
					}

					// Verify content actually changed (avoid duplicates)
					newHash := c.hashContent(content)

					c.mu.Lock()
					if newHash != c.lastHash {
						c.lastHash = newHash
						c.lastChangeCount = currentCount
						c.sequenceNumber.Add(1)
						c.lastModified = time.Now()
						c.mu.Unlock()

						// Reset idle count and speed up polling
						idleCount = 0
						ticker.Reset(500 * time.Millisecond)

						// Send notification (non-blocking to prevent deadlock)
						select {
						case ch <- struct{}{}:
							// Successfully sent
						case <-ctx.Done():
							return
						default:
							// Channel full - skip notification
							if os.Getenv("CI") == "true" {
								fmt.Printf("[DarwinClipboard.Watch] Dropped clipboard notification - channel full\n")
							}
						}
					} else {
						c.lastChangeCount = currentCount
						c.mu.Unlock()
					}
				} else {
					// No change, increment idle count
					idleCount++
					if idleCount > maxIdleCount {
						// Slow down polling when idle
						ticker.Reset(2 * time.Second)
					}
				}
			}
		}
	}()

	return ch
}

// getChangeCount retrieves the macOS clipboard change count using AppleScript.
// The changeCount is a property of NSPasteboard that increments each time
// the clipboard content changes. This provides a fast way to detect changes
// without reading the actual clipboard content.
//
// Returns -1 if the changeCount cannot be retrieved (e.g., AppleScript error).
func (c *DarwinClipboard) getChangeCount() int {
	// Use a shorter timeout for this quick operation
	config := &CommandConfig{
		Timeout:       2 * time.Second,
		MaxOutputSize: 1024, // changeCount output is tiny
		Logger:        c.cmdConfig.Logger,
	}

	// Use AppleScript to get the pasteboard change count
	script := `
		use framework "AppKit"
		set pb to current application's NSPasteboard's generalPasteboard()
		return pb's changeCount() as integer
	`

	output, err := RunCommand("osascript", []string{"-e", script}, config)
	if err != nil {
		// Fallback to -1 to indicate error
		return -1
	}

	countStr := strings.TrimSpace(string(output))
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return -1
	}

	return count
}

// hashContent creates a SHA-256 hash of the content for change detection.
// This is used to verify that clipboard content has actually changed,
// as changeCount can increment even when setting the same content.
func (c *DarwinClipboard) hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// GetState returns current state information
func (c *DarwinClipboard) GetState() ClipboardState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ClipboardState{
		SequenceNumber: c.sequenceNumber.Load(),
		LastModified:   c.lastModified,
		ContentHash:    c.lastHash,
	}
}
