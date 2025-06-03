package mesh

import (
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	t.Run("DefaultBackoff", func(t *testing.T) {
		b := DefaultBackoff()

		// Check defaults
		if b.initial != time.Second {
			t.Errorf("initial = %v, want %v", b.initial, time.Second)
		}
		if b.max != 5*time.Minute {
			t.Errorf("max = %v, want %v", b.max, 5*time.Minute)
		}
		if b.factor != 2.0 {
			t.Errorf("factor = %v, want %v", b.factor, 2.0)
		}
		if b.jitter != 0.1 {
			t.Errorf("jitter = %v, want %v", b.jitter, 0.1)
		}
	})

	t.Run("ExponentialProgression", func(t *testing.T) {
		b := NewExponentialBackoff(100*time.Millisecond, time.Second, 2.0, 0)

		// First attempt
		d1 := b.Next()
		if d1 != 100*time.Millisecond {
			t.Errorf("1st duration = %v, want %v", d1, 100*time.Millisecond)
		}

		// Second attempt (2x)
		d2 := b.Next()
		if d2 != 200*time.Millisecond {
			t.Errorf("2nd duration = %v, want %v", d2, 200*time.Millisecond)
		}

		// Third attempt (4x)
		d3 := b.Next()
		if d3 != 400*time.Millisecond {
			t.Errorf("3rd duration = %v, want %v", d3, 400*time.Millisecond)
		}

		// Fourth attempt (8x)
		d4 := b.Next()
		if d4 != 800*time.Millisecond {
			t.Errorf("4th duration = %v, want %v", d4, 800*time.Millisecond)
		}

		// Fifth attempt (would be 16x but capped at max)
		d5 := b.Next()
		if d5 != time.Second {
			t.Errorf("5th duration = %v, want %v (max)", d5, time.Second)
		}

		// Sixth attempt (still at max)
		d6 := b.Next()
		if d6 != time.Second {
			t.Errorf("6th duration = %v, want %v (max)", d6, time.Second)
		}
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
			min := 500 * time.Millisecond
			max := 1500 * time.Millisecond
			if d < min || d > max {
				t.Errorf("duration %v outside jitter range [%v, %v]", d, min, max)
			}
		}

		// Should see some variation
		if len(seen) < 2 {
			t.Error("jitter not producing varied results")
		}
	})

	t.Run("Reset", func(t *testing.T) {
		b := NewExponentialBackoff(100*time.Millisecond, time.Second, 2.0, 0)

		// Advance a few times
		b.Next()
		b.Next()
		b.Next()

		if b.Attempts() != 3 {
			t.Errorf("attempts = %d, want 3", b.Attempts())
		}

		// Reset
		b.Reset()

		if b.Attempts() != 0 {
			t.Errorf("attempts after reset = %d, want 0", b.Attempts())
		}

		// Next should be initial again
		d := b.Next()
		if d != 100*time.Millisecond {
			t.Errorf("duration after reset = %v, want %v", d, 100*time.Millisecond)
		}
	})

	t.Run("Duration", func(t *testing.T) {
		b := NewExponentialBackoff(100*time.Millisecond, time.Second, 2.0, 0)

		// Duration should return current without advancing
		d1 := b.Duration()
		if d1 != 100*time.Millisecond {
			t.Errorf("initial duration = %v, want %v", d1, 100*time.Millisecond)
		}

		// Still the same
		d2 := b.Duration()
		if d2 != 100*time.Millisecond {
			t.Errorf("duration (no advance) = %v, want %v", d2, 100*time.Millisecond)
		}

		// Advance
		b.Next()

		// Now should be doubled
		d3 := b.Duration()
		if d3 != 200*time.Millisecond {
			t.Errorf("duration after Next = %v, want %v", d3, 200*time.Millisecond)
		}
	})

	t.Run("InvalidParameters", func(t *testing.T) {
		// Test parameter validation
		tests := []struct {
			name    string
			initial time.Duration
			max     time.Duration
			factor  float64
			jitter  float64
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
				
				if tt.wantInitial != 0 && b.initial != tt.wantInitial {
					t.Errorf("initial = %v, want %v", b.initial, tt.wantInitial)
				}
				if tt.wantMax != 0 && b.max != tt.wantMax {
					t.Errorf("max = %v, want %v", b.max, tt.wantMax)
				}
				if tt.wantFactor != 0 && b.factor != tt.wantFactor {
					t.Errorf("factor = %v, want %v", b.factor, tt.wantFactor)
				}
				if tt.wantJitter != 0 && b.jitter != tt.wantJitter {
					t.Errorf("jitter = %v, want %v", b.jitter, tt.wantJitter)
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
		if b1 == nil {
			t.Fatal("Get returned nil")
		}

		// Second get returns same instance
		b2 := m.Get("test")
		if b1 != b2 {
			t.Error("Get returned different instance")
		}
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
		if b2.Attempts() != 0 {
			t.Error("backoffs not independent")
		}
	})

	t.Run("Reset", func(t *testing.T) {
		m := newBackoffManager(DefaultBackoff)

		b := m.Get("test")
		b.Next()
		b.Next()

		// Reset via manager
		m.Reset("test")

		if b.Attempts() != 0 {
			t.Error("Reset didn't reset backoff")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		m := newBackoffManager(DefaultBackoff)

		b1 := m.Get("test")
		b1.Next() // Advance it

		// Remove
		m.Remove("test")

		// Get again should be new instance at initial state
		b2 := m.Get("test")
		if b1 == b2 {
			t.Error("Remove didn't remove backoff")
		}
		if b2.Attempts() != 0 {
			t.Error("new backoff not at initial state")
		}
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
		if b1.Attempts() != 0 {
			t.Error("Clear didn't clear all backoffs")
		}
	})

	t.Run("NilFactory", func(t *testing.T) {
		m := newBackoffManager(nil)

		b := m.Get("test")
		if b == nil {
			t.Fatal("Get with nil factory returned nil")
		}

		// Should use DefaultBackoff
		if b.initial != time.Second {
			t.Error("nil factory didn't use DefaultBackoff")
		}
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
			if got != tt.expected {
				t.Errorf("calculateBackoffDuration() = %v, want %v", got, tt.expected)
			}
		})
	}
}