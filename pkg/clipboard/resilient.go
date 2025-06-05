// Package clipboard resilient implementation
//
// This file provides a resilient clipboard wrapper that adds production hardening
// features like retry logic, rate limiting, and graceful degradation.

package clipboard

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ResilientClipboard wraps a platform clipboard with additional hardening features
type ResilientClipboard struct {
	clipboard    Clipboard
	retryConfig  *RetryConfig
	rateLimiter  *RateLimiter
	mu           sync.RWMutex
	lastError    error
	errorCount   int
	fallbackMode bool
}

// NewResilientClipboard creates a new resilient clipboard wrapper
func NewResilientClipboard(clipboard Clipboard) *ResilientClipboard {
	return &ResilientClipboard{
		clipboard:   clipboard,
		retryConfig: DefaultRetryConfig(),
		rateLimiter: NewRateLimiter(100, time.Minute), // 100 operations per minute
	}
}

// Read returns the current clipboard contents with retry logic
func (rc *ResilientClipboard) Read() (string, error) {
	// Check rate limit
	if !rc.rateLimiter.Allow() {
		return "", fmt.Errorf("clipboard read rate limit exceeded")
	}

	// Check if we're in fallback mode
	rc.mu.RLock()
	inFallback := rc.fallbackMode
	rc.mu.RUnlock()

	if inFallback {
		return "", fmt.Errorf("clipboard temporarily unavailable due to repeated failures")
	}

	var result string
	err := RetryOperation(context.Background(), rc.retryConfig, func() error {
		var readErr error
		result, readErr = rc.clipboard.Read()
		return readErr
	})

	rc.updateErrorState(err)
	return result, err
}

// Write sets the clipboard contents with retry logic
func (rc *ResilientClipboard) Write(content string) error {
	// Check rate limit
	if !rc.rateLimiter.Allow() {
		return fmt.Errorf("clipboard write rate limit exceeded")
	}

	// Check if we're in fallback mode
	rc.mu.RLock()
	inFallback := rc.fallbackMode
	rc.mu.RUnlock()

	if inFallback {
		return fmt.Errorf("clipboard temporarily unavailable due to repeated failures")
	}

	err := RetryOperation(context.Background(), rc.retryConfig, func() error {
		return rc.clipboard.Write(content)
	})

	rc.updateErrorState(err)
	return err
}

// Watch monitors clipboard changes with automatic reconnection on failures
func (rc *ResilientClipboard) Watch(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, 10)

	go func() {
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Create a sub-context for this watch attempt
			watchCtx, watchCancel := context.WithCancel(ctx)

			// Start watching
			watchCh := rc.clipboard.Watch(watchCtx)

			// Forward notifications
			functioning := true
			for functioning {
				select {
				case <-ctx.Done():
					watchCancel()
					return
				case notification, ok := <-watchCh:
					if !ok {
						// Channel closed, need to reconnect
						functioning = false
						break
					}
					// Forward the notification
					select {
					case ch <- notification:
					case <-ctx.Done():
						watchCancel()
						return
					default:
						// Channel full, skip
					}
				}
			}

			watchCancel()

			// Wait before reconnecting
			select {
			case <-time.After(time.Second):
				// Continue to reconnect
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch
}

// GetState returns current state information
func (rc *ResilientClipboard) GetState() ClipboardState {
	return rc.clipboard.GetState()
}

// updateErrorState tracks errors and manages fallback mode
func (rc *ResilientClipboard) updateErrorState(err error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if err == nil {
		// Reset error state on success
		rc.errorCount = 0
		rc.lastError = nil
		rc.fallbackMode = false
		return
	}

	rc.lastError = err
	rc.errorCount++

	// Enter fallback mode after 5 consecutive failures
	if rc.errorCount >= 5 {
		rc.fallbackMode = true
		// Schedule recovery attempt
		go rc.scheduleRecovery()
	}
}

// scheduleRecovery attempts to recover from fallback mode
func (rc *ResilientClipboard) scheduleRecovery() {
	time.Sleep(30 * time.Second)

	// Try a test operation
	_, err := rc.clipboard.Read()

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if err == nil {
		// Recovery successful
		rc.errorCount = 0
		rc.fallbackMode = false
		rc.lastError = nil
	}
}

// GetErrorState returns the current error state for monitoring
func (rc *ResilientClipboard) GetErrorState() (int, bool, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.errorCount, rc.fallbackMode, rc.lastError
}

// SetRetryConfig updates the retry configuration
func (rc *ResilientClipboard) SetRetryConfig(config *RetryConfig) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.retryConfig = config
}

// SetRateLimiter updates the rate limiter
func (rc *ResilientClipboard) SetRateLimiter(limiter *RateLimiter) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.rateLimiter = limiter
}
