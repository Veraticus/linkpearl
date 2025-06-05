package mesh

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventTypeString tests the String method of EventType
func TestEventTypeString(t *testing.T) {
	tests := []struct {
		name     string
		event    EventType
		expected string
	}{
		{
			name:     "peer connected",
			event:    PeerConnected,
			expected: "PeerConnected",
		},
		{
			name:     "peer disconnected",
			event:    PeerDisconnected,
			expected: "PeerDisconnected",
		},
		{
			name:     "unknown event",
			event:    EventType(999),
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.event.String()
			assert.Equal(t, tt.expected, got, "EventType.String()")
		})
	}
}

// TestEventBufferCreation tests creating event buffers with various capacities
func TestEventBufferCreation(t *testing.T) {
	tests := []struct {
		name        string
		capacity    int
		expectedCap int
	}{
		{
			name:        "positive capacity",
			capacity:    100,
			expectedCap: 100,
		},
		{
			name:        "zero capacity defaults to 1000",
			capacity:    0,
			expectedCap: 1000,
		},
		{
			name:        "negative capacity defaults to 1000",
			capacity:    -10,
			expectedCap: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := newEventBuffer(tt.capacity)
			assert.Equal(t, tt.expectedCap, buf.cap, "newEventBuffer(%d).cap", tt.capacity)
			assert.Equal(t, 0, buf.size, "newEventBuffer(%d).size", tt.capacity)
		})
	}
}

// TestEventBufferPushPop tests basic push and pop operations
func TestEventBufferPushPop(t *testing.T) {
	buf := newEventBuffer(10)

	// Test empty pop
	_, ok := buf.Pop()
	assert.False(t, ok, "Pop() on empty buffer should return false")

	// Test single push/pop
	event1 := TopologyEvent{
		Type: PeerConnected,
		Peer: Node{ID: "node1", Mode: "full", Addr: ":8080"},
		Time: time.Now(),
	}
	buf.Push(event1)

	assert.Equal(t, 1, buf.Size(), "Size() after push")

	popped, ok := buf.Pop()
	assert.True(t, ok, "Pop() should return true when buffer has items")
	assert.Equal(t, event1.Peer.ID, popped.Peer.ID, "Pop() returned wrong event")

	// Buffer should be empty again
	assert.Equal(t, 0, buf.Size(), "Size() after pop")
}

// TestEventBufferOverflow tests ring buffer overflow behavior
func TestEventBufferOverflow(t *testing.T) {
	capacity := 5
	buf := newEventBuffer(capacity)

	// Push more events than capacity
	events := make([]TopologyEvent, capacity+3)
	for i := range events {
		events[i] = TopologyEvent{
			Type: PeerConnected,
			Peer: Node{ID: string(rune('A' + i)), Mode: "full", Addr: ":8080"},
			Time: time.Now(),
		}
		buf.Push(events[i])
	}

	// Buffer should be at capacity
	assert.Equal(t, capacity, buf.Size(), "Size() after overflow")

	// Oldest events should have been dropped
	// Should contain events D, E, F, G, H (indices 3-7)
	slice := buf.ToSlice()
	assert.Len(t, slice, capacity, "ToSlice() length")

	// Verify we have the newest events
	for i := 0; i < capacity; i++ {
		expectedID := string(rune('A' + 3 + i)) // Starting from 'D'
		assert.Equal(t, expectedID, slice[i].Peer.ID, "Event %d peer ID", i)
	}
}

// TestEventBufferClear tests clearing the buffer
func TestEventBufferClear(t *testing.T) {
	buf := newEventBuffer(10)

	// Add some events
	for i := 0; i < 5; i++ {
		buf.Push(TopologyEvent{
			Type: PeerConnected,
			Peer: Node{ID: string(rune('A' + i)), Mode: "full", Addr: ":8080"},
			Time: time.Now(),
		})
	}

	assert.Equal(t, 5, buf.Size(), "Size() before clear")

	buf.Clear()

	assert.Equal(t, 0, buf.Size(), "Size() after clear")
	assert.Nil(t, buf.ToSlice(), "ToSlice() after clear should return nil")
}

// TestEventBufferToSlice tests converting buffer to slice
func TestEventBufferToSlice(t *testing.T) {
	buf := newEventBuffer(10)

	// Empty buffer should return nil
	assert.Nil(t, buf.ToSlice(), "ToSlice() on empty buffer should return nil")

	// Add events
	events := []TopologyEvent{
		{Type: PeerConnected, Peer: Node{ID: "A", Mode: "full", Addr: ":8080"}, Time: time.Now()},
		{Type: PeerDisconnected, Peer: Node{ID: "B", Mode: "client", Addr: ""}, Time: time.Now()},
		{Type: PeerConnected, Peer: Node{ID: "C", Mode: "full", Addr: ":8081"}, Time: time.Now()},
	}

	for _, event := range events {
		buf.Push(event)
	}

	slice := buf.ToSlice()
	assert.Len(t, slice, len(events), "ToSlice() length")

	// Verify order is preserved
	for i, event := range slice {
		assert.Equal(t, events[i].Peer.ID, event.Peer.ID, "Event %d peer ID", i)
	}
}

// TestEventBufferConcurrentOperations tests thread safety
func TestEventBufferConcurrentOperations(t *testing.T) {
	buf := newEventBuffer(100)
	numGoroutines := 10
	numOperations := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // pushers, poppers, and readers

	// Start pushers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				buf.Push(TopologyEvent{
					Type: PeerConnected,
					Peer: Node{ID: string(rune('A' + id)), Mode: "full", Addr: ":8080"},
					Time: time.Now(),
				})
			}
		}(i)
	}

	// Start poppers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				buf.Pop()
			}
		}()
	}

	// Start readers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				buf.ToSlice()
				buf.Size()
			}
		}()
	}

	wg.Wait()

	// Buffer should still be in a consistent state
	size := buf.Size()
	slice := buf.ToSlice()
	assert.True(t, size >= 0 && size <= buf.cap, "Invalid size after concurrent operations: %d", size)
	if size == 0 {
		assert.Nil(t, slice, "ToSlice() returned non-nil for empty buffer")
	} else {
		assert.Equal(t, size, len(slice), "ToSlice() length doesn't match Size()")
	}
}

// TestEventPumpCreation tests creating event pumps
func TestEventPumpCreation(t *testing.T) {
	pump := newEventPump(100)
	require.NotNil(t, pump.buffer, "Event pump buffer should not be nil")
	assert.False(t, pump.closed, "New event pump should not be closed")
	assert.Empty(t, pump.listeners, "New event pump should have no listeners")
}

// TestEventPumpSubscribeUnsubscribe tests subscription management
func TestEventPumpSubscribeUnsubscribe(t *testing.T) {
	pump := newEventPump(100)
	ch1 := make(chan TopologyEvent, 1)
	ch2 := make(chan TopologyEvent, 1)

	// Subscribe channels
	pump.Subscribe(ch1)
	pump.Subscribe(ch2)

	assert.Len(t, pump.listeners, 2, "Expected 2 listeners")

	// Unsubscribe ch1
	pump.Unsubscribe(ch1)

	assert.Len(t, pump.listeners, 1, "Expected 1 listener after unsubscribe")

	// Unsubscribe non-existent channel should not panic
	ch3 := make(chan TopologyEvent)
	pump.Unsubscribe(ch3)

	// Unsubscribe ch2
	pump.Unsubscribe(ch2)

	assert.Empty(t, pump.listeners, "Expected 0 listeners after all unsubscribed")
}

// TestEventPumpPublish tests publishing events to subscribers
func TestEventPumpPublish(t *testing.T) {
	pump := newEventPump(100)
	ch1 := make(chan TopologyEvent, 10)
	ch2 := make(chan TopologyEvent, 10)

	pump.Subscribe(ch1)
	pump.Subscribe(ch2)

	event := TopologyEvent{
		Type: PeerConnected,
		Peer: Node{ID: "test-node", Mode: "full", Addr: ":8080"},
		Time: time.Now(),
	}

	pump.Publish(event)

	// Both channels should receive the event
	select {
	case received := <-ch1:
		assert.Equal(t, event.Peer.ID, received.Peer.ID, "ch1 received wrong event")
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1 did not receive event")
	}

	select {
	case received := <-ch2:
		assert.Equal(t, event.Peer.ID, received.Peer.ID, "ch2 received wrong event")
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2 did not receive event")
	}

	// Verify event was buffered
	assert.Equal(t, 1, pump.buffer.Size(), "Expected 1 event in buffer")
}

// TestEventPumpNonBlockingPublish tests that publish doesn't block on full channels
func TestEventPumpNonBlockingPublish(t *testing.T) {
	pump := newEventPump(100)

	// Create a channel with no buffer
	ch := make(chan TopologyEvent)
	pump.Subscribe(ch)

	// This should not block
	done := make(chan bool)
	go func() {
		pump.Publish(TopologyEvent{
			Type: PeerConnected,
			Peer: Node{ID: "test", Mode: "full", Addr: ":8080"},
			Time: time.Now(),
		})
		done <- true
	}()

	select {
	case <-done:
		// Good, publish didn't block
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "Publish blocked on full channel")
	}
}

// TestEventPumpClose tests closing the event pump
func TestEventPumpClose(t *testing.T) {
	pump := newEventPump(100)
	ch := make(chan TopologyEvent, 1)
	pump.Subscribe(ch)

	// Add an event to the buffer
	pump.Publish(TopologyEvent{
		Type: PeerConnected,
		Peer: Node{ID: "test", Mode: "full", Addr: ":8080"},
		Time: time.Now(),
	})

	pump.Close()

	assert.True(t, pump.closed, "Event pump should be marked as closed")
	assert.Empty(t, pump.listeners, "Listeners should be cleared after close")
	assert.Equal(t, 0, pump.buffer.Size(), "Buffer should be cleared after close")

	// Closing again should not panic
	pump.Close()

	// Publishing after close should not panic
	pump.Publish(TopologyEvent{
		Type: PeerDisconnected,
		Peer: Node{ID: "test2", Mode: "client", Addr: ""},
		Time: time.Now(),
	})

	// Subscribing after close should not add listener
	ch2 := make(chan TopologyEvent)
	pump.Subscribe(ch2)
	assert.Empty(t, pump.listeners, "Subscribe after close should not add listener")
}

// TestEventPumpConcurrentOperations tests thread safety of event pump
func TestEventPumpConcurrentOperations(t *testing.T) {
	pump := newEventPump(1000)
	numPublishers := 10
	numSubscribers := 10
	numEvents := 100

	var wg sync.WaitGroup
	var received atomic.Int64

	// Create subscribers
	channels := make([]chan TopologyEvent, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		ch := make(chan TopologyEvent, numEvents*numPublishers)
		channels[i] = ch
		pump.Subscribe(ch)

		// Start consumer
		wg.Add(1)
		go func(ch <-chan TopologyEvent, i int) {
			defer wg.Done()
			timeout := time.After(200 * time.Millisecond)
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					received.Add(1)
				case <-timeout:
					return
				}
			}
		}(ch, i)
	}

	// Start publishers
	wg.Add(numPublishers)
	for i := 0; i < numPublishers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numEvents; j++ {
				pump.Publish(TopologyEvent{
					Type: PeerConnected,
					Peer: Node{ID: string(rune('A' + id)), Mode: "full", Addr: ":8080"},
					Time: time.Now(),
				})
			}
		}(i)
	}

	// Let publishers finish
	time.Sleep(100 * time.Millisecond)

	// Close pump (this will close subscriber channels)
	pump.Close()

	wg.Wait()

	// We should have received many events (exact count depends on timing)
	assert.NotZero(t, received.Load(), "No events were received")
}

// TestEventOrdering tests that events maintain order within a single publisher
func TestEventOrdering(t *testing.T) {
	pump := newEventPump(1000)
	ch := make(chan TopologyEvent, 100)
	pump.Subscribe(ch)

	// Publish events with increasing timestamps
	baseTime := time.Now()
	numEvents := 50
	for i := 0; i < numEvents; i++ {
		pump.Publish(TopologyEvent{
			Type: PeerConnected,
			Peer: Node{ID: string(rune('A' + i)), Mode: "full", Addr: ":8080"},
			Time: baseTime.Add(time.Duration(i) * time.Millisecond),
		})
	}

	// Verify events are received in order
	lastTime := time.Time{}
	received := 0
	for i := 0; i < numEvents; i++ {
		select {
		case event := <-ch:
			if !lastTime.IsZero() {
				assert.False(t, event.Time.Before(lastTime), "Event %d has timestamp before previous event", i)
			}
			lastTime = event.Time
			received++
		case <-time.After(100 * time.Millisecond):
			require.Fail(t, "Timeout waiting for event %d", i)
		}
	}

	assert.Equal(t, numEvents, received, "Received events count")
}

// TestEventPumpWithMultipleCloseAndSubscribe tests edge cases
func TestEventPumpWithMultipleCloseAndSubscribe(t *testing.T) {
	pump := newEventPump(100)

	// Subscribe, close, then try to subscribe again
	ch1 := make(chan TopologyEvent)
	pump.Subscribe(ch1)
	pump.Close()

	ch2 := make(chan TopologyEvent)
	pump.Subscribe(ch2)

	// Should have no listeners after close
	assert.Empty(t, pump.listeners, "Should have no listeners after close")

	// Unsubscribe after close should not panic
	pump.Unsubscribe(ch1)
	pump.Unsubscribe(ch2)
}

// TestRingBufferEdgeCases tests edge cases for the ring buffer
func TestRingBufferEdgeCases(t *testing.T) {
	// Test with capacity of 1
	buf := newEventBuffer(1)

	event1 := TopologyEvent{Type: PeerConnected, Peer: Node{ID: "A", Mode: "full", Addr: ":8080"}, Time: time.Now()}
	event2 := TopologyEvent{Type: PeerDisconnected, Peer: Node{ID: "B", Mode: "client", Addr: ""}, Time: time.Now()}

	buf.Push(event1)
	buf.Push(event2) // Should overwrite event1

	assert.Equal(t, 1, buf.Size(), "Buffer size")

	popped, ok := buf.Pop()
	assert.True(t, ok, "Pop should succeed")
	assert.Equal(t, "B", popped.Peer.ID, "Should have popped event2")

	// Test wraparound multiple times
	buf2 := newEventBuffer(3)
	for i := 0; i < 10; i++ {
		buf2.Push(TopologyEvent{
			Type: PeerConnected,
			Peer: Node{ID: string(rune('A' + i)), Mode: "full", Addr: ":8080"},
			Time: time.Now(),
		})
	}

	// Should contain the last 3 events: H, I, J
	slice := buf2.ToSlice()
	require.Len(t, slice, 3, "Expected 3 events")

	expectedIDs := []string{"H", "I", "J"}
	for i, event := range slice {
		assert.Equal(t, expectedIDs[i], event.Peer.ID, "Event %d ID", i)
	}
}

// BenchmarkEventBufferPush benchmarks push operations
func BenchmarkEventBufferPush(b *testing.B) {
	buf := newEventBuffer(1000)
	event := TopologyEvent{
		Type: PeerConnected,
		Peer: Node{ID: "bench-node", Mode: "full", Addr: ":8080"},
		Time: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Push(event)
	}
}

// BenchmarkEventPumpPublish benchmarks publish operations
func BenchmarkEventPumpPublish(b *testing.B) {
	pump := newEventPump(1000)

	// Add some subscribers
	for i := 0; i < 10; i++ {
		ch := make(chan TopologyEvent, 100)
		pump.Subscribe(ch)
		go func() {
			for range ch {
				// Drain channel
			}
		}()
	}

	event := TopologyEvent{
		Type: PeerConnected,
		Peer: Node{ID: "bench-node", Mode: "full", Addr: ":8080"},
		Time: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pump.Publish(event)
	}
}
