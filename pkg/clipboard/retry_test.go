package clipboard

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestRetryConfig_isRetryableError(t *testing.T) {
	config := DefaultRetryConfig()

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
		{
			name:      "deadline exceeded",
			err:       context.DeadlineExceeded,
			retryable: true,
		},
		{
			name:      "not supported error",
			err:       ErrNotSupported,
			retryable: false,
		},
		{
			name:      "content too large error",
			err:       ErrContentTooLarge,
			retryable: false,
		},
		{
			name:      "temporary failure",
			err:       errors.New("temporary failure in operation"),
			retryable: true,
		},
		{
			name:      "resource temporarily unavailable",
			err:       errors.New("resource temporarily unavailable"),
			retryable: true,
		},
		{
			name:      "generic error",
			err:       errors.New("some other error"),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.isRetryableError(tt.err)
			if got != tt.retryable {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

func TestRetryConfig_calculateDelay(t *testing.T) {
	config := &RetryConfig{
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      2 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0, // No jitter for predictable tests
	}

	tests := []struct {
		attempt       int
		expectedDelay time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
		{6, 2000 * time.Millisecond}, // Capped at MaxDelay
		{7, 2000 * time.Millisecond}, // Still capped
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.attempt)), func(t *testing.T) {
			got := config.calculateDelay(tt.attempt)
			if got != tt.expectedDelay {
				t.Errorf("calculateDelay(%d) = %v, want %v", tt.attempt, got, tt.expectedDelay)
			}
		})
	}
}

func TestRetryConfig_calculateDelayWithJitter(t *testing.T) {
	config := &RetryConfig{
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      2 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.3,
	}

	// Test that jitter produces values within expected range
	for attempt := 1; attempt <= 5; attempt++ {
		baseDelay := float64(config.InitialDelay) * math.Pow(config.BackoffFactor, float64(attempt-1))
		if baseDelay > float64(config.MaxDelay) {
			baseDelay = float64(config.MaxDelay)
		}

		minDelay := time.Duration(baseDelay * (1 - config.JitterFactor))
		maxDelay := time.Duration(baseDelay * (1 + config.JitterFactor))

		// Run multiple times to test randomness
		for i := 0; i < 10; i++ {
			got := config.calculateDelay(attempt)
			if got < minDelay || got > maxDelay {
				t.Errorf("calculateDelay(%d) = %v, want between %v and %v",
					attempt, got, minDelay, maxDelay)
			}
		}
	}
}

func TestRetryWithBackoff(t *testing.T) {
	t.Run("immediate success", func(t *testing.T) {
		attempts := 0
		err := RetryWithBackoff(context.Background(), func() error {
			attempts++
			return nil
		})

		if err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("eventual success", func(t *testing.T) {
		attempts := 0
		err := RetryWithBackoff(context.Background(), func() error {
			attempts++
			if attempts < 2 {
				return context.DeadlineExceeded
			}
			return nil
		})

		if err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}
		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})
}

func TestRetryOperation_Timing(t *testing.T) {
	config := &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  50 * time.Millisecond,
		MaxDelay:      200 * time.Millisecond,
		BackoffFactor: 2.0,
		JitterFactor:  0,
	}

	attempts := 0
	start := time.Now()

	err := RetryOperation(context.Background(), config, func() error {
		attempts++
		if attempts < 3 {
			return context.DeadlineExceeded
		}
		return nil
	})

	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected nil error, got: %v", err)
	}

	// Expected delays: 50ms after attempt 1, 100ms after attempt 2
	// Total expected: ~150ms (plus some execution time)
	minExpected := 150 * time.Millisecond
	maxExpected := 200 * time.Millisecond

	if elapsed < minExpected || elapsed > maxExpected {
		t.Errorf("Expected elapsed time between %v and %v, got %v",
			minExpected, maxExpected, elapsed)
	}
}

func TestRetryOperation_NonRetryableError(t *testing.T) {
	config := DefaultRetryConfig()
	attempts := 0

	err := RetryOperation(context.Background(), config, func() error {
		attempts++
		return ErrNotSupported // Non-retryable error
	})

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempts)
	}
}
