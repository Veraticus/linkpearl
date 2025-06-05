package clipboard

import (
	"testing"
	"time"
)

func TestRateLimiter_BasicFunctionality(t *testing.T) {
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

	// Check tokens available
	tokens := limiter.TokensAvailable()
	// Use tolerance for float comparison
	if tokens > 0.01 {
		t.Errorf("Expected ~0 tokens, got %f", tokens)
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	limiter := NewRateLimiter(10, time.Second)

	// Use all tokens
	for i := 0; i < 10; i++ {
		limiter.Allow()
	}

	// No tokens should be available
	if limiter.Allow() {
		t.Error("Should have no tokens available")
	}

	// Wait for partial refill (half a second = 5 tokens)
	time.Sleep(500 * time.Millisecond)

	// Should have approximately 5 tokens
	tokens := limiter.TokensAvailable()
	if tokens < 4.5 || tokens > 5.5 {
		t.Errorf("Expected ~5 tokens after 500ms, got %f", tokens)
	}

	// Should allow 5 operations
	allowed := 0
	for i := 0; i < 10; i++ {
		if limiter.Allow() {
			allowed++
		}
	}
	if allowed != 5 {
		t.Errorf("Expected to allow 5 operations, allowed %d", allowed)
	}
}

func TestRateLimiter_MaxTokensCap(t *testing.T) {
	limiter := NewRateLimiter(5, 100*time.Millisecond)

	// Wait for a long time
	time.Sleep(500 * time.Millisecond)

	// Tokens should be capped at max
	tokens := limiter.TokensAvailable()
	if tokens != 5.0 {
		t.Errorf("Expected tokens to be capped at 5, got %f", tokens)
	}

	// Should still only allow 5 operations
	allowed := 0
	for i := 0; i < 10; i++ {
		if limiter.Allow() {
			allowed++
		}
	}
	if allowed != 5 {
		t.Errorf("Expected to allow 5 operations, allowed %d", allowed)
	}
}

func TestRateLimiter_AllowN(t *testing.T) {
	limiter := NewRateLimiter(10, time.Second)

	// Should allow taking 5 tokens at once
	if !limiter.AllowN(5) {
		t.Error("Should allow taking 5 tokens")
	}

	// Should have 5 tokens left
	tokens := limiter.TokensAvailable()
	// Use tolerance for float comparison
	if tokens < 4.99 || tokens > 5.01 {
		t.Errorf("Expected ~5 tokens remaining, got %f", tokens)
	}

	// Should not allow taking 6 tokens
	if limiter.AllowN(6) {
		t.Error("Should not allow taking 6 tokens when only 5 available")
	}

	// Should still allow taking 5 tokens
	if !limiter.AllowN(5) {
		t.Error("Should allow taking remaining 5 tokens")
	}

	// Should have 0 tokens left
	tokens = limiter.TokensAvailable()
	// Use tolerance for float comparison
	if tokens > 0.01 {
		t.Errorf("Expected ~0 tokens remaining, got %f", tokens)
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	limiter := NewRateLimiter(10, time.Second)

	// Use some tokens
	limiter.AllowN(7)

	// Should have 3 tokens left
	tokens := limiter.TokensAvailable()
	// Use tolerance for float comparison
	if tokens < 2.99 || tokens > 3.01 {
		t.Errorf("Expected ~3 tokens before reset, got %f", tokens)
	}

	// Reset
	limiter.Reset()

	// Should have full tokens
	tokens = limiter.TokensAvailable()
	if tokens != 10.0 {
		t.Errorf("Expected 10 tokens after reset, got %f", tokens)
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewRateLimiter(100, time.Second)

	// Run concurrent operations
	done := make(chan bool, 10)
	allowed := make(chan bool, 100)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				if limiter.Allow() {
					allowed <- true
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	close(allowed)

	// Count total allowed operations
	totalAllowed := 0
	for range allowed {
		totalAllowed++
	}

	// Should have allowed exactly 100 operations
	if totalAllowed != 100 {
		t.Errorf("Expected 100 operations allowed, got %d", totalAllowed)
	}
}

func TestRateLimiter_RefillRate(t *testing.T) {
	// 60 operations per minute = 1 per second
	limiter := NewRateLimiter(60, time.Minute)

	// Use all tokens
	limiter.AllowN(60)

	// Wait 2 seconds
	time.Sleep(2 * time.Second)

	// Should have approximately 2 tokens
	tokens := limiter.TokensAvailable()
	if tokens < 1.8 || tokens > 2.2 {
		t.Errorf("Expected ~2 tokens after 2 seconds, got %f", tokens)
	}
}
