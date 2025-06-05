package mesh

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExponentialBackoff(t *testing.T) {
	t.Run("DefaultBackoff", func(t *testing.T) {
		b := DefaultBackoff()

		// Check defaults
		assert.Equal(t, time.Second, b.initial)
		assert.Equal(t, 5*time.Minute, b.max)
		assert.Equal(t, 2.0, b.factor)
		assert.Equal(t, 0.1, b.jitter)
	})

	t.Run("ExponentialProgression", func(t *testing.T) {
		b := NewExponentialBackoff(100*time.Millisecond, time.Second, 2.0, 0)

		// First attempt
		d1 := b.Next()
		assert.Equal(t, 100*time.Millisecond, d1, "1st duration")

		// Second attempt (2x)
		d2 := b.Next()
		assert.Equal(t, 200*time.Millisecond, d2, "2nd duration")

		// Third attempt (4x)
		d3 := b.Next()
		assert.Equal(t, 400*time.Millisecond, d3, "3rd duration")

		// Fourth attempt (8x)
		d4 := b.Next()
		assert.Equal(t, 800*time.Millisecond, d4, "4th duration")

		// Fifth attempt (would be 16x but capped at max)
		d5 := b.Next()
		assert.Equal(t, time.Second, d5, "5th duration (max)")

		// Sixth attempt (still at max)
		d6 := b.Next()
		assert.Equal(t, time.Second, d6, "6th duration (max)")
	})

	t.Run("Jitter", func(t *testing.T) {
		b := NewExponentialBackoff(time.Second, 5*time.Second, 2.0, 0.5)

		// Get multiple samples
		seen := make(map[time.Duration]bool)
		for i := 0; i < 20; i++ {
			b.Reset()
			d := b.Next()
			seen[d] = true

			// Check within jitter range (±50%)
			minTime := 500 * time.Millisecond
			maxTime := 1500 * time.Millisecond
			assert.GreaterOrEqual(t, d, minTime, "duration should be >= min")
			assert.LessOrEqual(t, d, maxTime, "duration should be <= max")
		}

		// Should see some variation
		assert.GreaterOrEqual(t, len(seen), 2, "jitter not producing varied results")
	})

	t.Run("Reset", func(t *testing.T) {
		b := NewExponentialBackoff(100*time.Millisecond, time.Second, 2.0, 0)

		// Advance a few times
		b.Next()
		b.Next()
		b.Next()

		assert.Equal(t, 3, b.Attempts())

		// Reset
		b.Reset()

		assert.Equal(t, 0, b.Attempts(), "attempts after reset")

		// Next should be initial again
		d := b.Next()
		assert.Equal(t, 100*time.Millisecond, d, "duration after reset")
	})

	t.Run("Duration", func(t *testing.T) {
		b := NewExponentialBackoff(100*time.Millisecond, time.Second, 2.0, 0)

		// Duration should return current without advancing
		d1 := b.Duration()
		assert.Equal(t, 100*time.Millisecond, d1, "initial duration")

		// Still the same
		d2 := b.Duration()
		assert.Equal(t, 100*time.Millisecond, d2, "duration (no advance)")

		// Advance
		b.Next()

		// Now should be doubled
		d3 := b.Duration()
		assert.Equal(t, 200*time.Millisecond, d3, "duration after Next")
	})

	t.Run("InvalidParameters", func(t *testing.T) {
		// Test parameter validation
		tests := []struct {
			name        string
			initial     time.Duration
			max         time.Duration
			factor      float64
			jitter      float64
			wantInitial time.Duration
			wantMax     time.Duration
			wantFactor  float64
			wantJitter  float64
		}{
			{
				name:        "negative initial",
				initial:     -1,
				max:         5 * time.Minute,
				factor:      2.0,
				jitter:      0.1,
				wantInitial: time.Second,
			},
			{
				name:    "negative max",
				initial: time.Second,
				max:     -1,
				factor:  2.0,
				jitter:  0.1,
				wantMax: 5 * time.Minute,
			},
			{
				name:       "factor too small",
				initial:    time.Second,
				max:        5 * time.Minute,
				factor:     0.5,
				jitter:     0.1,
				wantFactor: 2.0,
			},
			{
				name:       "negative jitter",
				initial:    time.Second,
				max:        5 * time.Minute,
				factor:     2.0,
				jitter:     -0.5,
				wantJitter: 0.1,
			},
			{
				name:       "jitter too large",
				initial:    time.Second,
				max:        5 * time.Minute,
				factor:     2.0,
				jitter:     1.5,
				wantJitter: 0.1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				b := NewExponentialBackoff(tt.initial, tt.max, tt.factor, tt.jitter)

				if tt.wantInitial != 0 {
					assert.Equal(t, tt.wantInitial, b.initial, "initial")
				}
				if tt.wantMax != 0 {
					assert.Equal(t, tt.wantMax, b.max, "max")
				}
				if tt.wantFactor != 0 {
					assert.Equal(t, tt.wantFactor, b.factor, "factor")
				}
				if tt.wantJitter != 0 {
					assert.Equal(t, tt.wantJitter, b.jitter, "jitter")
				}
			})
		}
	})
}

func TestBackoffManager(t *testing.T) {
	t.Run("GetCreatesNew", func(t *testing.T) {
		m := newBackoffManager(func() *ExponentialBackoff {
			return NewExponentialBackoff(10*time.Millisecond, 100*time.Millisecond, 2.0, 0)
		})

		// First get creates new
		b1 := m.Get("test")
		require.NotNil(t, b1, "Get returned nil")

		// Second get returns same instance
		b2 := m.Get("test")
		assert.Equal(t, b1, b2, "Get returned different instance")
	})

	t.Run("MultipleKeys", func(t *testing.T) {
		m := newBackoffManager(DefaultBackoff)

		b1 := m.Get("key1")
		b2 := m.Get("key2")

		if b1 == b2 {
			t.Error("different keys returned same backoff")
		}

		// Advance one
		b1.Next()
		b1.Next()

		// Other should still be at initial
		assert.Equal(t, 0, b2.Attempts(), "backoffs not independent")
	})

	t.Run("Reset", func(t *testing.T) {
		m := newBackoffManager(DefaultBackoff)

		b := m.Get("test")
		b.Next()
		b.Next()

		// Reset via manager
		m.Reset("test")

		assert.Equal(t, 0, b.Attempts(), "Reset didn't reset backoff")
	})

	t.Run("Remove", func(t *testing.T) {
		m := newBackoffManager(DefaultBackoff)

		b1 := m.Get("test")
		b1.Next() // Advance it

		// Remove
		m.Remove("test")

		// Get again should be new instance at initial state
		b2 := m.Get("test")
		assert.NotEqual(t, b1, b2, "Remove didn't remove backoff")
		assert.Equal(t, 0, b2.Attempts(), "new backoff not at initial state")
	})

	t.Run("Clear", func(t *testing.T) {
		m := newBackoffManager(DefaultBackoff)

		// Create several
		m.Get("key1")
		m.Get("key2")
		m.Get("key3")

		// Clear all
		m.Clear()

		// All should be new
		b1 := m.Get("key1")
		assert.Equal(t, 0, b1.Attempts(), "Clear didn't clear all backoffs")
	})

	t.Run("NilFactory", func(t *testing.T) {
		m := newBackoffManager(nil)

		b := m.Get("test")
		require.NotNil(t, b, "Get with nil factory returned nil")

		// Should use DefaultBackoff
		assert.Equal(t, time.Second, b.initial, "nil factory didn't use DefaultBackoff")
	})
}

func TestCalculateBackoffDuration(t *testing.T) {
	tests := []struct {
		name     string
		attempt  int
		initial  time.Duration
		max      time.Duration
		factor   float64
		expected time.Duration
	}{
		{
			name:     "attempt 0",
			attempt:  0,
			initial:  time.Second,
			max:      time.Minute,
			factor:   2.0,
			expected: time.Second,
		},
		{
			name:     "attempt 1",
			attempt:  1,
			initial:  time.Second,
			max:      time.Minute,
			factor:   2.0,
			expected: time.Second,
		},
		{
			name:     "attempt 2",
			attempt:  2,
			initial:  time.Second,
			max:      time.Minute,
			factor:   2.0,
			expected: 2 * time.Second,
		},
		{
			name:     "attempt 3",
			attempt:  3,
			initial:  time.Second,
			max:      time.Minute,
			factor:   2.0,
			expected: 4 * time.Second,
		},
		{
			name:     "capped at max",
			attempt:  10,
			initial:  time.Second,
			max:      time.Minute,
			factor:   2.0,
			expected: time.Minute,
		},
		{
			name:     "factor 3",
			attempt:  3,
			initial:  100 * time.Millisecond,
			max:      time.Minute,
			factor:   3.0,
			expected: 900 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateBackoffDuration(tt.attempt, tt.initial, tt.max, tt.factor)
			assert.Equal(t, tt.expected, got, "calculateBackoffDuration()")
		})
	}
}
