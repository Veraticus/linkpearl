package mesh

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
			if got != tt.expected {
				t.Errorf("EventType.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestEventBufferCreation tests creating event buffers with various capacities
func TestEventBufferCreation(t *testing.T) {
	tests := []struct {
		name         string
		capacity     int
		expectedCap  int
	}{
		{
			name:         "positive capacity",
			capacity:     100,
			expectedCap:  100,
		},
		{
			name:         "zero capacity defaults to 1000",
			capacity:     0,
			expectedCap:  1000,
		},
		{
			name:         "negative capacity defaults to 1000",
			capacity:     -10,
			expectedCap:  1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := newEventBuffer(tt.capacity)
			if buf.cap != tt.expectedCap {
				t.Errorf("newEventBuffer(%d).cap = %d, want %d", tt.capacity, buf.cap, tt.expectedCap)
			}
			if buf.size != 0 {
				t.Errorf("newEventBuffer(%d).size = %d, want 0", tt.capacity, buf.size)
			}
		})
	}
}

// TestEventBufferPushPop tests basic push and pop operations
func TestEventBufferPushPop(t *testing.T) {
	buf := newEventBuffer(10)

	// Test empty pop
	if _, ok := buf.Pop(); ok {
		t.Error("Pop() on empty buffer should return false")
	}

	// Test single push/pop
	event1 := TopologyEvent{
		Type: PeerConnected,
		Peer: Node{ID: "node1", Mode: "full", Addr: ":8080"},
		Time: time.Now(),
	}
	buf.Push(event1)

	if buf.Size() != 1 {
		t.Errorf("Size() = %d, want 1", buf.Size())
	}

	popped, ok := buf.Pop()
	if !ok {
		t.Error("Pop() should return true when buffer has items")
	}
	if popped.Peer.ID != event1.Peer.ID {
		t.Errorf("Pop() returned wrong event, got peer ID %s, want %s", popped.Peer.ID, event1.Peer.ID)
	}

	// Buffer should be empty again
	if buf.Size() != 0 {
		t.Errorf("Size() = %d after pop, want 0", buf.Size())
	}
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
	if buf.Size() != capacity {
		t.Errorf("Size() = %d, want %d", buf.Size(), capacity)
	}

	// Oldest events should have been dropped
	// Should contain events D, E, F, G, H (indices 3-7)
	slice := buf.ToSlice()
	if len(slice) != capacity {
		t.Errorf("ToSlice() returned %d events, want %d", len(slice), capacity)
	}

	// Verify we have the newest events
	for i := 0; i < capacity; i++ {
		expectedID := string(rune('A' + 3 + i)) // Starting from 'D'
		if slice[i].Peer.ID != expectedID {
			t.Errorf("Event %d has peer ID %s, want %s", i, slice[i].Peer.ID, expectedID)
		}
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

	if buf.Size() != 5 {
		t.Errorf("Size() before clear = %d, want 5", buf.Size())
	}

	buf.Clear()

	if buf.Size() != 0 {
		t.Errorf("Size() after clear = %d, want 0", buf.Size())
	}

	if slice := buf.ToSlice(); slice != nil {
		t.Error("ToSlice() after clear should return nil")
	}
}

// TestEventBufferToSlice tests converting buffer to slice
func TestEventBufferToSlice(t *testing.T) {
	buf := newEventBuffer(10)

	// Empty buffer should return nil
	if slice := buf.ToSlice(); slice != nil {
		t.Error("ToSlice() on empty buffer should return nil")
	}

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
	if len(slice) != len(events) {
		t.Errorf("ToSlice() returned %d events, want %d", len(slice), len(events))
	}

	// Verify order is preserved
	for i, event := range slice {
		if event.Peer.ID != events[i].Peer.ID {
			t.Errorf("Event %d has peer ID %s, want %s", i, event.Peer.ID, events[i].Peer.ID)
		}
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
	if size < 0 || size > buf.cap {
		t.Errorf("Invalid size after concurrent operations: %d", size)
	}
	if size == 0 && slice != nil {
		t.Error("ToSlice() returned non-nil for empty buffer")
	}
	if size > 0 && len(slice) != size {
		t.Errorf("ToSlice() length %d doesn't match Size() %d", len(slice), size)
	}
}

// TestEventPumpCreation tests creating event pumps
func TestEventPumpCreation(t *testing.T) {
	pump := newEventPump(100)
	if pump.buffer == nil {
		t.Error("Event pump buffer should not be nil")
	}
	if pump.closed {
		t.Error("New event pump should not be closed")
	}
	if len(pump.listeners) != 0 {
		t.Error("New event pump should have no listeners")
	}
}

// TestEventPumpSubscribeUnsubscribe tests subscription management
func TestEventPumpSubscribeUnsubscribe(t *testing.T) {
	pump := newEventPump(100)
	ch1 := make(chan TopologyEvent, 1)
	ch2 := make(chan TopologyEvent, 1)

	// Subscribe channels
	pump.Subscribe(ch1)
	pump.Subscribe(ch2)

	if len(pump.listeners) != 2 {
		t.Errorf("Expected 2 listeners, got %d", len(pump.listeners))
	}

	// Unsubscribe ch1
	pump.Unsubscribe(ch1)

	if len(pump.listeners) != 1 {
		t.Errorf("Expected 1 listener after unsubscribe, got %d", len(pump.listeners))
	}

	// Unsubscribe non-existent channel should not panic
	ch3 := make(chan TopologyEvent)
	pump.Unsubscribe(ch3)

	// Unsubscribe ch2
	pump.Unsubscribe(ch2)

	if len(pump.listeners) != 0 {
		t.Errorf("Expected 0 listeners after all unsubscribed, got %d", len(pump.listeners))
	}
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
		if received.Peer.ID != event.Peer.ID {
			t.Errorf("ch1 received wrong event, got peer ID %s, want %s", received.Peer.ID, event.Peer.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1 did not receive event")
	}

	select {
	case received := <-ch2:
		if received.Peer.ID != event.Peer.ID {
			t.Errorf("ch2 received wrong event, got peer ID %s, want %s", received.Peer.ID, event.Peer.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2 did not receive event")
	}

	// Verify event was buffered
	if pump.buffer.Size() != 1 {
		t.Errorf("Expected 1 event in buffer, got %d", pump.buffer.Size())
	}
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
		t.Error("Publish blocked on full channel")
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

	if !pump.closed {
		t.Error("Event pump should be marked as closed")
	}

	if len(pump.listeners) != 0 {
		t.Error("Listeners should be cleared after close")
	}

	if pump.buffer.Size() != 0 {
		t.Error("Buffer should be cleared after close")
	}

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
	if len(pump.listeners) != 0 {
		t.Error("Subscribe after close should not add listener")
	}
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
		go func(ch <-chan TopologyEvent) {
			defer wg.Done()
			for range ch {
				received.Add(1)
			}
		}(ch)
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

	// Close pump and channels
	pump.Close()
	for _, ch := range channels {
		close(ch)
	}

	wg.Wait()

	// We should have received many events (exact count depends on timing)
	if received.Load() == 0 {
		t.Error("No events were received")
	}
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
			if !lastTime.IsZero() && event.Time.Before(lastTime) {
				t.Errorf("Event %d has timestamp before previous event", i)
			}
			lastTime = event.Time
			received++
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for event %d", i)
		}
	}

	if received != numEvents {
		t.Errorf("Received %d events, expected %d", received, numEvents)
	}
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
	if len(pump.listeners) != 0 {
		t.Error("Should have no listeners after close")
	}

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
	
	if buf.Size() != 1 {
		t.Errorf("Buffer size = %d, want 1", buf.Size())
	}
	
	popped, ok := buf.Pop()
	if !ok || popped.Peer.ID != "B" {
		t.Error("Should have popped event2")
	}

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
	if len(slice) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(slice))
	}
	
	expectedIDs := []string{"H", "I", "J"}
	for i, event := range slice {
		if event.Peer.ID != expectedIDs[i] {
			t.Errorf("Event %d has ID %s, want %s", i, event.Peer.ID, expectedIDs[i])
		}
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