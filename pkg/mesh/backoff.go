// backoff.go implements exponential backoff strategies for connection retry logic.
// This file provides a robust backoff implementation that prevents the mesh from
// overwhelming peers with rapid reconnection attempts during failures.
//
// The backoff algorithm uses exponential growth with configurable parameters:
//   - Initial delay: The starting delay between attempts
//   - Maximum delay: The ceiling for backoff duration
//   - Growth factor: The multiplier applied after each failure
//   - Jitter: Random variation to prevent synchronized reconnection storms
//
// The implementation includes automatic reset logic that returns to the initial
// delay after a period of successful operation, ensuring that temporary failures
// don't permanently impact reconnection speed.

package mesh

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// ExponentialBackoff implements an exponential backoff strategy with jitter.
// This is used to manage reconnection attempts in a way that provides quick
// initial retries while avoiding overwhelming the target with repeated attempts
// during extended failures.
//
// The backoff duration grows exponentially with each failure, up to a maximum
// value. Jitter is added to prevent multiple nodes from synchronizing their
// reconnection attempts, which could cause thundering herd problems.
type ExponentialBackoff struct {
	initial time.Duration
	max     time.Duration
	factor  float64
	jitter  float64

	mu        sync.Mutex
	current   time.Duration
	attempts  int
	resetTime time.Time
}

// NewExponentialBackoff creates a new exponential backoff
func NewExponentialBackoff(initial, max time.Duration, factor, jitter float64) *ExponentialBackoff {
	if initial <= 0 {
		initial = time.Second
	}
	if max <= 0 {
		max = 5 * time.Minute
	}
	if factor <= 1 {
		factor = 2.0
	}
	if jitter < 0 || jitter > 1 {
		jitter = 0.1
	}

	return &ExponentialBackoff{
		initial: initial,
		max:     max,
		factor:  factor,
		jitter:  jitter,
		current: initial,
	}
}

// DefaultBackoff returns a backoff with default settings
func DefaultBackoff() *ExponentialBackoff {
	return NewExponentialBackoff(
		time.Second,      // 1s initial
		5*time.Minute,    // 5m max
		2.0,              // 2x factor
		0.1,              // 10% jitter
	)
}

// Next returns the next backoff duration
func (b *ExponentialBackoff) Next() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Calculate base duration
	duration := b.current

	// Apply jitter
	if b.jitter > 0 {
		jitterRange := float64(duration) * b.jitter
		jitterValue := (rand.Float64()*2 - 1) * jitterRange // -jitter to +jitter
		duration = time.Duration(float64(duration) + jitterValue)
	}

	// Update for next time
	b.attempts++
	b.current = time.Duration(float64(b.current) * b.factor)
	if b.current > b.max {
		b.current = b.max
	}

	// Record when we started backing off
	if b.attempts == 1 {
		b.resetTime = time.Now().Add(b.max * 2) // Reset after 2x max duration of no calls
	}

	return duration
}

// Reset resets the backoff to initial state
func (b *ExponentialBackoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.current = b.initial
	b.attempts = 0
	b.resetTime = time.Time{}
}

// Attempts returns the number of attempts since last reset
func (b *ExponentialBackoff) Attempts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.attempts
}

// Duration returns the current backoff duration without advancing
func (b *ExponentialBackoff) Duration() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.current
}

// ShouldReset returns true if the backoff should be reset due to inactivity
func (b *ExponentialBackoff) ShouldReset() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.attempts == 0 {
		return false
	}

	// Reset if we haven't been called in a while
	return !b.resetTime.IsZero() && time.Now().After(b.resetTime)
}

// backoffManager manages multiple backoffs by key.
// This is used by the topology to maintain separate backoff states for each
// peer address, ensuring that failures connecting to one peer don't affect
// the reconnection timing for other peers.
//
// The manager automatically creates new backoff instances as needed and
// provides thread-safe access to them. It also handles cleanup of unused
// backoff states to prevent memory leaks in dynamic topologies.
type backoffManager struct {
	backoffs map[string]*ExponentialBackoff
	factory  func() *ExponentialBackoff
	mu       sync.RWMutex
}

// newBackoffManager creates a new backoff manager
func newBackoffManager(factory func() *ExponentialBackoff) *backoffManager {
	if factory == nil {
		factory = DefaultBackoff
	}
	return &backoffManager{
		backoffs: make(map[string]*ExponentialBackoff),
		factory:  factory,
	}
}

// Get returns the backoff for the given key, creating it if necessary
func (m *backoffManager) Get(key string) *ExponentialBackoff {
	m.mu.RLock()
	backoff, exists := m.backoffs[key]
	m.mu.RUnlock()

	if exists {
		// Check if we should reset due to inactivity
		if backoff.ShouldReset() {
			backoff.Reset()
		}
		return backoff
	}

	// Create new backoff
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if backoff, exists := m.backoffs[key]; exists {
		return backoff
	}

	backoff = m.factory()
	m.backoffs[key] = backoff
	return backoff
}

// Reset resets the backoff for the given key
func (m *backoffManager) Reset(key string) {
	m.mu.RLock()
	backoff, exists := m.backoffs[key]
	m.mu.RUnlock()

	if exists {
		backoff.Reset()
	}
}

// Remove removes the backoff for the given key
func (m *backoffManager) Remove(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.backoffs, key)
}

// Clear removes all backoffs
func (m *backoffManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backoffs = make(map[string]*ExponentialBackoff)
}

// calculateBackoffDuration is a helper for testing specific backoff calculations
func calculateBackoffDuration(attempt int, initial, max time.Duration, factor float64) time.Duration {
	if attempt <= 0 {
		return initial
	}

	duration := initial * time.Duration(math.Pow(factor, float64(attempt-1)))
	if duration > max {
		return max
	}
	return duration
}