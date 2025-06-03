//go:build darwin
// +build darwin

package clipboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DarwinClipboard implements clipboard access on macOS using pbcopy/pbpaste
type DarwinClipboard struct {
	mu              sync.RWMutex
	lastHash        string
	lastChangeCount int
}

// newPlatformClipboard returns a macOS clipboard implementation
func newPlatformClipboard() (Clipboard, error) {
	return &DarwinClipboard{}, nil
}

// Read returns the current clipboard contents
func (c *DarwinClipboard) Read() (string, error) {
	cmd := exec.Command("pbpaste")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read clipboard: %w", err)
	}
	return string(output), nil
}

// Write sets the clipboard contents
func (c *DarwinClipboard) Write(content string) error {
	cmd := exec.Command("pbcopy")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start pbcopy: %w", err)
	}

	if _, err := stdin.Write([]byte(content)); err != nil {
		stdin.Close()
		cmd.Process.Kill()
		return fmt.Errorf("failed to write to clipboard: %w", err)
	}

	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pbcopy failed: %w", err)
	}

	// Update our tracking state
	c.mu.Lock()
	c.lastHash = c.hashContent(content)
	c.lastChangeCount = c.getChangeCount()
	c.mu.Unlock()

	return nil
}

// Watch monitors the clipboard for changes using macOS changeCount
func (c *DarwinClipboard) Watch(ctx context.Context) <-chan string {
	ch := make(chan string)

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
						c.mu.Unlock()

						// Reset idle count and speed up polling
						idleCount = 0
						ticker.Reset(500 * time.Millisecond)

						// Send the new content
						select {
						case ch <- content:
						case <-ctx.Done():
							return
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

// getChangeCount retrieves the macOS clipboard change count
func (c *DarwinClipboard) getChangeCount() int {
	// Use AppleScript to get the pasteboard change count
	cmd := exec.Command("osascript", "-e", `
		use framework "AppKit"
		set pb to current application's NSPasteboard's generalPasteboard()
		return pb's changeCount() as integer
	`)

	output, err := cmd.Output()
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

// hashContent creates a hash of the content for change detection
func (c *DarwinClipboard) hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
