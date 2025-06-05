//go:build linux
// +build linux

// This file implements clipboard access for Linux systems supporting both X11 and Wayland.
//
// Implementation Details:
//
// The Linux implementation supports multiple clipboard tools to ensure compatibility
// across different desktop environments and display servers. It automatically detects
// and uses the first available tool from a prioritized list.
//
// Supported Clipboard Tools (in order of preference):
//   1. wl-clipboard (wl-paste/wl-copy) - Wayland native clipboard utilities
//   2. xsel - X11 clipboard utility with good performance
//   3. xclip - X11 clipboard utility, widely available fallback
//
// Tool Selection:
//
// During initialization, the implementation checks for available tools in order
// of preference. The first available tool is selected and used for all operations.
// Special handling is provided for systems that have wl-copy but not wl-paste.
//
// Change Detection:
//
// The Watch method supports two monitoring strategies:
//   1. Event-based: Uses clipnotify if available for efficient monitoring
//   2. Polling-based: Falls back to smart polling with adaptive intervals
//
// Clipnotify Integration:
//
// When clipnotify is available, it provides efficient event-based monitoring
// by listening to X11 clipboard events. This eliminates the need for polling
// and provides instant change notifications. If clipnotify fails or is not
// available, the implementation automatically falls back to polling.
//
// Smart Polling:
//
// The polling implementation uses adaptive intervals:
//   - Active: 500ms (when changes are detected)
//   - Idle: 2s (after 10 consecutive polls with no changes)
//
// Content Normalization:
//
// Linux clipboard tools may handle line endings differently. The implementation
// normalizes line endings (CRLF -> LF) before hashing to ensure consistent
// change detection across different tools and environments.
//
// Error Handling:
//
// The implementation handles various edge cases:
//   - Empty clipboard (exit code 1 from some tools)
//   - Missing clipboard tools (returns appropriate error)
//   - Tool-specific quirks and error codes
//
// Thread Safety:
//
// Uses sync.RWMutex to protect shared state, ensuring thread-safe access
// from multiple goroutines.

package clipboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LinuxClipboard implements clipboard access on Linux using various tools.
// It automatically detects and uses the appropriate clipboard utility based
// on what's available on the system (Wayland or X11).
type LinuxClipboard struct {
	mu             sync.RWMutex
	lastHash       string        // SHA-256 hash of last known clipboard content
	tool           clipboardTool // The selected clipboard tool for this instance
	sequenceNumber atomic.Uint64
	lastModified   time.Time
	cmdConfig      *CommandConfig // Configuration for command execution
}

// clipboardTool represents a clipboard utility and its command-line arguments
type clipboardTool struct {
	name      string   // Command name (e.g., "xsel", "wl-paste")
	readArgs  []string // Arguments for reading clipboard
	writeArgs []string // Arguments for writing clipboard
	available bool     // Whether the tool is available on the system
}

// clipboardTools defines the supported clipboard utilities in order of preference.
// The first available tool will be used. Wayland tools are preferred over X11 tools
// when both are available.
var clipboardTools = []clipboardTool{
	{
		name:      "wl-paste",
		readArgs:  []string{"--no-newline"},
		writeArgs: []string{},
	},
	{
		name:      "xsel",
		readArgs:  []string{"--output", "--clipboard"},
		writeArgs: []string{"--input", "--clipboard"},
	},
	{
		name:      "xclip",
		readArgs:  []string{"-out", "-selection", "clipboard"},
		writeArgs: []string{"-in", "-selection", "clipboard"},
	},
}

// newPlatformClipboard returns a Linux clipboard implementation.
// It automatically detects available clipboard tools and selects the most
// appropriate one based on the system configuration. Returns an error if
// no supported clipboard tool is found.
func newPlatformClipboard() (Clipboard, error) {
	c := &LinuxClipboard{}

	// Find the first available clipboard tool
	for _, tool := range clipboardTools {
		if _, err := exec.LookPath(tool.name); err == nil {
			tool.available = true
			c.tool = tool
			break
		}
	}

	if !c.tool.available {
		// If wl-paste is not found, check for wl-copy (Wayland)
		if _, err := exec.LookPath("wl-copy"); err == nil {
			c.tool = clipboardTool{
				name:      "wl-copy",
				writeArgs: []string{},
				available: true,
			}
			// For reading, we still need wl-paste
			if _, err := exec.LookPath("wl-paste"); err == nil {
				c.tool.readArgs = []string{"--no-newline"}
			}
		}
	}

	if !c.tool.available || (c.tool.name == "wl-copy" && len(c.tool.readArgs) == 0) {
		return nil, fmt.Errorf("no clipboard tool found (install xsel, xclip, or wl-clipboard)")
	}

	c.cmdConfig = DefaultCommandConfig()
	return c, nil
}

// Read returns the current clipboard contents using the selected tool.
// Handles special cases like empty clipboard (which may return exit code 1)
// and the wl-copy/wl-paste split in Wayland environments.
// The operation has a 5-second timeout to prevent hanging on system issues.
// Content size is checked to prevent memory issues with extremely large clipboard data.
func (c *LinuxClipboard) Read() (string, error) {
	// Handle special case where we have wl-copy but need wl-paste for reading
	readTool := c.tool.name
	if c.tool.name == "wl-copy" {
		readTool = "wl-paste"
	}

	output, err := RunCommand(readTool, c.tool.readArgs, c.cmdConfig)
	if err != nil {
		return "", fmt.Errorf("failed to read clipboard with %s: %w", readTool, err)
	}

	if err := ValidateContent(output); err != nil {
		return "", err
	}

	return string(output), nil
}

// Write sets the clipboard contents using the selected tool.
// The content is piped to the tool's stdin. After a successful write,
// the internal tracking state is updated with the content hash.
// The operation has a 5-second timeout to prevent hanging on system issues.
// Content size is limited to MaxClipboardSize to prevent memory issues.
func (c *LinuxClipboard) Write(content string) error {
	contentBytes := []byte(content)
	if err := ValidateContent(contentBytes); err != nil {
		return err
	}

	if err := RunCommandWithInput(c.tool.name, c.tool.writeArgs, contentBytes, c.cmdConfig); err != nil {
		return fmt.Errorf("clipboard write failed: %w", err)
	}

	// Update our tracking state only after successful write
	c.mu.Lock()
	c.lastHash = c.hashContent(content)
	c.sequenceNumber.Add(1)
	c.lastModified = time.Now()
	c.mu.Unlock()

	return nil
}

// Watch monitors the clipboard for changes and returns a channel that signals
// when clipboard state changes. The channel receives empty structs as notifications.
// Actual content must be retrieved using Read().
//
// It automatically selects the best monitoring strategy:
//   - Uses clipnotify for efficient event-based monitoring if available
//   - Falls back to smart polling with adaptive intervals otherwise
//
// The returned channel is closed when the context is cancelled.
func (c *LinuxClipboard) Watch(ctx context.Context) <-chan struct{} {
	// Use buffered channel to prevent blocking the watcher goroutine
	// Buffer size of 10 allows for burst of changes without blocking
	ch := make(chan struct{}, 10)

	go func() {
		defer close(ch)

		// Check if clipnotify is available for efficient monitoring
		hasClipnotify := false
		if _, err := exec.LookPath("clipnotify"); err == nil {
			hasClipnotify = true
		}

		if hasClipnotify {
			c.watchWithClipnotify(ctx, ch)
		} else {
			c.watchWithPolling(ctx, ch)
		}
	}()

	return ch
}

// watchWithClipnotify uses the clipnotify tool for efficient clipboard monitoring.
// Clipnotify blocks until a clipboard change event occurs, eliminating the need
// for polling. If clipnotify fails, this method automatically falls back to polling.
func (c *LinuxClipboard) watchWithClipnotify(ctx context.Context, ch chan<- struct{}) {
	// Initialize with current content
	if content, err := c.Read(); err == nil {
		c.mu.Lock()
		c.lastHash = c.hashContent(content)
		c.mu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// clipnotify blocks until clipboard changes
			// We don't use RunCommand here because clipnotify blocks indefinitely
			cmd := exec.CommandContext(ctx, "clipnotify")
			if err := cmd.Run(); err != nil {
				// If clipnotify fails, fall back to polling
				if ctx.Err() == nil {
					time.Sleep(time.Second)
					c.watchWithPolling(ctx, ch)
				}
				return
			}

			// Clipboard changed, read the new content
			content, err := c.Read()
			if err != nil {
				continue
			}

			// Check if content actually changed
			newHash := c.hashContent(content)

			c.mu.Lock()
			if newHash != c.lastHash {
				c.lastHash = newHash
				c.sequenceNumber.Add(1)
				c.lastModified = time.Now()
				c.mu.Unlock()

				// Send notification (non-blocking to prevent deadlock)
				select {
				case ch <- struct{}{}:
					// Successfully sent
				case <-ctx.Done():
					return
				default:
					// Channel full - skip notification
					// This prevents the watcher from blocking if the consumer is slow
				}
			} else {
				c.mu.Unlock()
			}
		}
	}
}

// watchWithPolling implements polling-based clipboard monitoring.
// Uses adaptive polling intervals that speed up when changes are detected
// and slow down during idle periods to reduce CPU usage.
func (c *LinuxClipboard) watchWithPolling(ctx context.Context, ch chan<- struct{}) {
	// Initialize with current content
	if content, err := c.Read(); err == nil {
		c.mu.Lock()
		c.lastHash = c.hashContent(content)
		c.mu.Unlock()
	}

	// Smart polling with adaptive intervals
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	idleCount := 0
	const maxIdleCount = 10

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			content, err := c.Read()
			if err != nil {
				continue
			}

			newHash := c.hashContent(content)

			c.mu.RLock()
			lastHash := c.lastHash
			c.mu.RUnlock()

			if newHash != lastHash {
				c.mu.Lock()
				c.lastHash = newHash
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
					// This prevents the watcher from blocking if the consumer is slow
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
}

// hashContent creates a SHA-256 hash of the content for change detection.
// Line endings are normalized (CRLF -> LF) before hashing to ensure
// consistent detection across different clipboard tools.
func (c *LinuxClipboard) hashContent(content string) string {
	// Normalize line endings before hashing
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// GetState returns current state information
func (c *LinuxClipboard) GetState() ClipboardState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ClipboardState{
		SequenceNumber: c.sequenceNumber.Load(),
		LastModified:   c.lastModified,
		ContentHash:    c.lastHash,
	}
}
