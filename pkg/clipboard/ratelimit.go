// Package clipboard rate limiting implementation
//
// This file provides rate limiting functionality for clipboard operations
// to prevent abuse and protect system resources.

package clipboard

import (
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
// maxOps: maximum operations allowed per period
// period: time period for the rate limit
func NewRateLimiter(maxOps int, period time.Duration) *RateLimiter {
	return &RateLimiter{
		tokens:     float64(maxOps),
		maxTokens:  float64(maxOps),
		refillRate: float64(maxOps) / period.Seconds(),
		lastRefill: time.Now(),
	}
}

// Allow checks if an operation is allowed under the rate limit.
func (rl *RateLimiter) Allow() bool {
	return rl.AllowN(1)
}

// AllowN checks if n operations are allowed under the rate limit.
func (rl *RateLimiter) AllowN(n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		return true
	}

	return false
}

// refill adds tokens based on time elapsed.
func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()

	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}

	rl.lastRefill = now
}

// TokensAvailable returns the current number of available tokens.
func (rl *RateLimiter) TokensAvailable() float64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()
	return rl.tokens
}

// Reset resets the rate limiter to full capacity.
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.tokens = rl.maxTokens
	rl.lastRefill = time.Now()
}
