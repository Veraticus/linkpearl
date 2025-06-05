package clipboard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestSizeLimit tests the clipboard size limit enforcement.
func TestSizeLimit(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		shouldError bool
	}{
		{
			name:        "Small content",
			content:     "Hello, world!",
			shouldError: false,
		},
		{
			name:        "Content at limit",
			content:     strings.Repeat("a", MaxClipboardSize),
			shouldError: false,
		},
		{
			name:        "Content exceeds limit",
			content:     strings.Repeat("a", MaxClipboardSize+1),
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockClipboard()
			err := mock.Write(tt.content)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error for oversized content, got nil")
				} else if !errors.Is(err, ErrContentTooLarge) {
					t.Errorf("Expected ErrContentTooLarge, got %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestRetryLogic tests the retry functionality.
func TestRetryLogic(t *testing.T) {
	t.Run("Successful retry", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary failure")
			}
			return nil
		}

		config := &RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  10 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			BackoffFactor: 2.0,
			JitterFactor:  0,
		}

		err := RetryOperation(context.Background(), config, operation)
		if err != nil {
			t.Errorf("Expected success after retries, got error: %v", err)
		}
		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("Max attempts exceeded", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return context.DeadlineExceeded // Use a retryable error
		}

		config := &RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  10 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			BackoffFactor: 2.0,
			JitterFactor:  0,
		}

		err := RetryOperation(context.Background(), config, operation)
		if err == nil {
			t.Error("Expected error after max attempts")
		}
		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0

		operation := func() error {
			attempts++
			if attempts == 2 {
				cancel() // Cancel during retry
			}
			return context.DeadlineExceeded // Use a retryable error
		}

		config := &RetryConfig{
			MaxAttempts:   5,
			InitialDelay:  50 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			BackoffFactor: 2.0,
			JitterFactor:  0,
		}

		err := RetryOperation(ctx, config, operation)
		if err == nil {
			t.Error("Expected error from context cancellation")
		}
		if !strings.Contains(err.Error(), "retry canceled") {
			t.Errorf("Expected cancellation error, got: %v", err)
		}
		// Should stop early due to cancellation
		if attempts > 3 {
			t.Errorf("Expected early termination, got %d attempts", attempts)
		}
	})
}

// TestRateLimiter tests the rate limiting functionality.
func TestRateLimiter(t *testing.T) {
	t.Run("Basic rate limiting", func(t *testing.T) {
		limiter := NewRateLimiter(5, 100*time.Millisecond)

		// Should allow first 5 operations
		for i := 0; i < 5; i++ {
			if !limiter.Allow() {
				t.Errorf("Operation %d should be allowed", i+1)
			}
		}

		// 6th operation should be denied
		if limiter.Allow() {
			t.Error("6th operation should be denied")
		}

		// Wait for refill
		time.Sleep(110 * time.Millisecond)

		// Should allow operations again
		if !limiter.Allow() {
			t.Error("Operation should be allowed after refill")
		}
	})

	t.Run("Token count", func(t *testing.T) {
		limiter := NewRateLimiter(10, time.Second)

		// Check initial tokens
		tokens := limiter.TokensAvailable()
		if tokens != 10.0 {
			t.Errorf("Expected 10 tokens, got %f", tokens)
		}

		// Use some tokens
		limiter.AllowN(3)
		tokens = limiter.TokensAvailable()
		// Use tolerance for float comparison
		if tokens < 6.99 || tokens > 7.01 {
			t.Errorf("Expected ~7 tokens after using 3, got %f", tokens)
		}

		// Reset
		limiter.Reset()
		tokens = limiter.TokensAvailable()
		if tokens != 10.0 {
			t.Errorf("Expected 10 tokens after reset, got %f", tokens)
		}
	})
}

// TestMetricsCollection tests the metrics collector.
func TestMetricsCollection(t *testing.T) {
	collector := NewDefaultMetricsCollector()

	// Record some operations
	collector.RecordOperation("read", 100*time.Millisecond, nil)
	collector.RecordOperation("read", 200*time.Millisecond, nil)
	collector.RecordOperation("read", 150*time.Millisecond, errors.New("test error"))

	collector.RecordOperation("write", 50*time.Millisecond, nil)
	collector.RecordOperation("write", 75*time.Millisecond, nil)

	// Record sizes
	collector.RecordSize("read", 1000)
	collector.RecordSize("read", 2000)
	collector.RecordSize("write", 500)

	// Record errors
	collector.RecordError("read", errors.New("test error"))
	collector.RecordTimeout("read")
	collector.RecordRateLimitHit("write")

	// Get metrics snapshot
	snapshot := collector.GetMetrics()

	// Verify read metrics
	readMetrics, ok := snapshot.Operations["read"]
	if !ok {
		t.Fatal("Expected read metrics")
	}
	if readMetrics.Count != 3 {
		t.Errorf("Expected 3 read operations, got %d", readMetrics.Count)
	}
	if readMetrics.ErrorCount != 1 {
		t.Errorf("Expected 1 read error, got %d", readMetrics.ErrorCount)
	}
	if readMetrics.MinTime != 100*time.Millisecond {
		t.Errorf("Expected min time 100ms, got %v", readMetrics.MinTime)
	}
	if readMetrics.MaxTime != 200*time.Millisecond {
		t.Errorf("Expected max time 200ms, got %v", readMetrics.MaxTime)
	}

	// Verify write metrics
	writeMetrics, ok := snapshot.Operations["write"]
	if !ok {
		t.Fatal("Expected write metrics")
	}
	if writeMetrics.Count != 2 {
		t.Errorf("Expected 2 write operations, got %d", writeMetrics.Count)
	}

	// Verify error counts
	if snapshot.Timeouts["read"] != 1 {
		t.Errorf("Expected 1 read timeout, got %d", snapshot.Timeouts["read"])
	}
	if snapshot.RateLimitHits["write"] != 1 {
		t.Errorf("Expected 1 write rate limit hit, got %d", snapshot.RateLimitHits["write"])
	}
}

// TestResilientClipboard tests the resilient clipboard wrapper.
func TestResilientClipboard(t *testing.T) {
	t.Run("Fallback mode after failures", func(t *testing.T) {
		// Create a failing mock clipboard
		failingMock := &failingClipboard{
			failUntil: 10, // Fail first 10 operations
		}

		resilient := NewResilientClipboard(failingMock)

		// First 4 operations should retry and eventually fail
		for i := 0; i < 4; i++ {
			_, err := resilient.Read()
			if err == nil {
				t.Errorf("Expected error on attempt %d", i+1)
			}
		}

		// 5th failure should trigger fallback mode
		_, err := resilient.Read()
		if err == nil {
			t.Error("Expected error in fallback mode")
		}

		// Check error state
		errorCount, inFallback, lastErr := resilient.GetErrorState()
		if lastErr == nil {
			t.Error("Expected last error to be set")
		}
		if errorCount < 5 {
			t.Errorf("Expected error count >= 5, got %d", errorCount)
		}
		if !inFallback {
			t.Error("Expected to be in fallback mode")
		}

		// Operations should fail immediately in fallback mode
		_, err = resilient.Read()
		if err == nil || !strings.Contains(err.Error(), "temporarily unavailable") {
			t.Errorf("Expected fallback error, got: %v", err)
		}
	})
}

// TestInstrumentedClipboard tests the instrumented clipboard wrapper.
func TestInstrumentedClipboard(t *testing.T) {
	mock := NewMockClipboard()
	collector := NewDefaultMetricsCollector()
	instrumented := NewInstrumentedClipboard(mock, collector)

	// Perform some operations
	testContent := "test content"
	err := instrumented.Write(testContent)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	content, err := instrumented.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if content != testContent {
		t.Errorf("Expected %q, got %q", testContent, content)
	}

	// Check metrics were recorded
	snapshot := collector.GetMetrics()

	readMetrics, ok := snapshot.Operations["read"]
	if !ok || readMetrics.Count != 1 {
		t.Error("Expected read metrics to be recorded")
	}

	writeMetrics, ok := snapshot.Operations["write"]
	if !ok || writeMetrics.Count != 1 {
		t.Error("Expected write metrics to be recorded")
	}

	// Test watcher metrics
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = instrumented.Watch(ctx)

	// Give time for watcher to start
	time.Sleep(50 * time.Millisecond)

	// Emit change
	mock.EmitChange("new content")

	// Check that sequence number was recorded
	time.Sleep(50 * time.Millisecond)
	snapshot = collector.GetMetrics()
	if snapshot.SequenceNumber == 0 {
		t.Error("Expected sequence number to be recorded")
	}
}

// TestHardenedClipboard tests the fully hardened clipboard.
func TestHardenedClipboard(t *testing.T) {
	t.Run("Default configuration", func(t *testing.T) {
		// This test can only run with a mock clipboard
		// In real usage, it would create a platform clipboard
		mock := NewMockClipboard()
		options := DefaultOptions()

		// We can't directly test NewHardenedClipboard because it creates
		// a platform clipboard, but we can test the layering manually
		var clipboard Clipboard = mock

		// Add resilient layer
		resilient := NewResilientClipboard(clipboard)
		limiter := NewRateLimiter(options.RateLimitOpsPerMinute, time.Minute)
		resilient.SetRateLimiter(limiter)
		clipboard = resilient

		// Add instrumented layer
		collector := NewDefaultMetricsCollector()
		clipboard = NewInstrumentedClipboard(clipboard, collector)

		// Test that all layers work together
		testContent := "hardened clipboard test"
		err := clipboard.Write(testContent)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		content, err := clipboard.Read()
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if content != testContent {
			t.Errorf("Expected %q, got %q", testContent, content)
		}

		// Verify metrics were collected
		if instrumented, ok := clipboard.(*InstrumentedClipboard); ok {
			snapshot := instrumented.GetMetrics().GetMetrics()
			if len(snapshot.Operations) == 0 {
				t.Error("Expected operations to be recorded")
			}
		}
	})
}

// failingClipboard is a mock that fails operations until a certain count.
type failingClipboard struct {
	MockClipboard
	failUntil int
	failCount int
}

func (f *failingClipboard) Read() (string, error) {
	f.failCount++
	if f.failCount <= f.failUntil {
		return "", errors.New("simulated failure")
	}
	return f.MockClipboard.Read()
}

func (f *failingClipboard) Write(content string) error {
	f.failCount++
	if f.failCount <= f.failUntil {
		return errors.New("simulated failure")
	}
	return f.MockClipboard.Write(content)
}
