// Package clipboard retry logic implementation
//
// This file provides retry functionality with exponential backoff for clipboard operations.
// It helps handle transient failures that may occur when accessing system clipboard.

package clipboard

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryConfig defines configuration for retry behavior.
type RetryConfig struct {
	MaxAttempts     int           // Maximum number of retry attempts (including initial attempt)
	InitialDelay    time.Duration // Initial delay between retries
	MaxDelay        time.Duration // Maximum delay between retries
	BackoffFactor   float64       // Exponential backoff multiplier
	JitterFactor    float64       // Jitter factor (0.0 to 1.0) to randomize delays
	RetryableErrors []error       // List of errors that should trigger a retry
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      2 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
		RetryableErrors: []error{
			context.DeadlineExceeded,
			// Add more retryable errors as needed
		},
	}
}

// isRetryableError checks if an error should trigger a retry.
func (rc *RetryConfig) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry on permanent errors
	if errors.Is(err, ErrNotSupported) || errors.Is(err, ErrContentTooLarge) {
		return false
	}

	// Check if it's a timeout error
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check configured retryable errors
	for _, retryableErr := range rc.RetryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}

	// Check for specific error patterns
	errStr := err.Error()
	// Retry on temporary failures
	if contains(errStr, "temporary failure") || contains(errStr, "resource temporarily unavailable") {
		return true
	}

	// Default: don't retry unknown errors
	return false
}

// calculateDelay calculates the next retry delay with exponential backoff and jitter.
func (rc *RetryConfig) calculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return rc.InitialDelay
	}

	// Calculate exponential backoff
	delay := float64(rc.InitialDelay) * math.Pow(rc.BackoffFactor, float64(attempt-1))

	// Apply max delay cap
	if delay > float64(rc.MaxDelay) {
		delay = float64(rc.MaxDelay)
	}

	// Add jitter
	if rc.JitterFactor > 0 {
		//nolint:gosec // math/rand is acceptable for retry jitter
		jitter := delay * rc.JitterFactor * (2*rand.Float64() - 1) // -jitter to +jitter
		delay += jitter
		if delay < 0 {
			delay = float64(rc.InitialDelay)
		}
	}

	return time.Duration(delay)
}

// RetryOperation executes an operation with retry logic.
func RetryOperation(ctx context.Context, config *RetryConfig, operation func() error) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Execute the operation
		err := operation()

		// Success
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if attempt >= config.MaxAttempts || !config.isRetryableError(err) {
			break
		}

		// Calculate delay for next attempt
		delay := config.calculateDelay(attempt)

		// Wait with context cancellation support
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		}
	}

	if lastErr != nil {
		return fmt.Errorf("operation failed after %d attempts: %w", config.MaxAttempts, lastErr)
	}

	return lastErr
}

// RetryWithBackoff is a convenience function for common retry scenarios.
func RetryWithBackoff(ctx context.Context, operation func() error) error {
	return RetryOperation(ctx, DefaultRetryConfig(), operation)
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
