//go:build linux
// +build linux

package clipboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// LinuxClipboard implements clipboard access on Linux using various tools
type LinuxClipboard struct {
	mu       sync.RWMutex
	lastHash string
	tool     clipboardTool
}

type clipboardTool struct {
	name      string
	readArgs  []string
	writeArgs []string
	available bool
}

// Supported clipboard tools in order of preference
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

// newPlatformClipboard returns a Linux clipboard implementation
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

	return c, nil
}

// Read returns the current clipboard contents
func (c *LinuxClipboard) Read() (string, error) {
	// Handle special case where we have wl-copy but need wl-paste for reading
	readTool := c.tool.name
	if c.tool.name == "wl-copy" {
		readTool = "wl-paste"
	}

	cmd := exec.Command(readTool, c.tool.readArgs...)
	output, err := cmd.Output()
	if err != nil {
		// Check if the error is because clipboard is empty
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("failed to read clipboard with %s: %w", readTool, err)
	}

	return string(output), nil
}

// Write sets the clipboard contents
func (c *LinuxClipboard) Write(content string) error {
	cmd := exec.Command(c.tool.name, c.tool.writeArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", c.tool.name, err)
	}

	if _, err := stdin.Write([]byte(content)); err != nil {
		stdin.Close()
		cmd.Process.Kill()
		return fmt.Errorf("failed to write to clipboard: %w", err)
	}

	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s failed: %w", c.tool.name, err)
	}

	// Update our tracking state
	c.mu.Lock()
	c.lastHash = c.hashContent(content)
	c.mu.Unlock()

	return nil
}

// Watch monitors the clipboard for changes
func (c *LinuxClipboard) Watch(ctx context.Context) <-chan string {
	ch := make(chan string)

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

// watchWithClipnotify uses the clipnotify tool for efficient clipboard monitoring
func (c *LinuxClipboard) watchWithClipnotify(ctx context.Context, ch chan<- string) {
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
				c.mu.Unlock()

				select {
				case ch <- content:
				case <-ctx.Done():
					return
				}
			} else {
				c.mu.Unlock()
			}
		}
	}
}

// watchWithPolling falls back to polling when clipnotify is not available
func (c *LinuxClipboard) watchWithPolling(ctx context.Context, ch chan<- string) {
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
				c.mu.Unlock()

				// Reset idle count and speed up polling
				idleCount = 0
				ticker.Reset(500 * time.Millisecond)

				select {
				case ch <- content:
				case <-ctx.Done():
					return
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

// hashContent creates a hash of the content for change detection
func (c *LinuxClipboard) hashContent(content string) string {
	// Normalize line endings before hashing
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}