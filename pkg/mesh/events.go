package mesh

import (
	"sync"
	"time"
)

// EventType represents the type of topology event
type EventType int

const (
	// PeerConnected indicates a peer has connected
	PeerConnected EventType = iota
	// PeerDisconnected indicates a peer has disconnected
	PeerDisconnected
)

// String returns the string representation of the event type
func (e EventType) String() string {
	switch e {
	case PeerConnected:
		return "PeerConnected"
	case PeerDisconnected:
		return "PeerDisconnected"
	default:
		return "Unknown"
	}
}

// TopologyEvent represents an event in the topology
type TopologyEvent struct {
	Type EventType
	Peer Node
	Time time.Time
}

// eventBuffer implements a ring buffer for topology events
type eventBuffer struct {
	events []TopologyEvent
	head   int
	tail   int
	size   int
	cap    int
	mu     sync.Mutex
}

// newEventBuffer creates a new event buffer with the given capacity
func newEventBuffer(capacity int) *eventBuffer {
	if capacity <= 0 {
		capacity = 1000 // Default capacity
	}
	return &eventBuffer{
		events: make([]TopologyEvent, capacity),
		cap:    capacity,
	}
}

// Push adds an event to the buffer
func (b *eventBuffer) Push(event TopologyEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add event at tail position
	b.events[b.tail] = event
	b.tail = (b.tail + 1) % b.cap

	if b.size < b.cap {
		b.size++
	} else {
		// Buffer is full, move head forward (drop oldest)
		b.head = (b.head + 1) % b.cap
	}
}

// Pop removes and returns the oldest event
func (b *eventBuffer) Pop() (TopologyEvent, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.size == 0 {
		return TopologyEvent{}, false
	}

	event := b.events[b.head]
	b.head = (b.head + 1) % b.cap
	b.size--

	return event, true
}

// Size returns the current number of events in the buffer
func (b *eventBuffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.size
}

// Clear removes all events from the buffer
func (b *eventBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.head = 0
	b.tail = 0
	b.size = 0
}

// ToSlice returns all events in the buffer as a slice
func (b *eventBuffer) ToSlice() []TopologyEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.size == 0 {
		return nil
	}

	result := make([]TopologyEvent, b.size)
	for i := 0; i < b.size; i++ {
		idx := (b.head + i) % b.cap
		result[i] = b.events[idx]
	}

	return result
}

// eventPump manages event distribution to subscribers
type eventPump struct {
	buffer    *eventBuffer
	listeners []chan<- TopologyEvent
	mu        sync.RWMutex
	closed    bool
	wg        sync.WaitGroup
}

// newEventPump creates a new event pump
func newEventPump(bufferSize int) *eventPump {
	return &eventPump{
		buffer:    newEventBuffer(bufferSize),
		listeners: make([]chan<- TopologyEvent, 0),
	}
}

// Subscribe adds a listener for events
func (p *eventPump) Subscribe(ch chan<- TopologyEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.closed {
		p.listeners = append(p.listeners, ch)
	}
}

// Unsubscribe removes a listener
func (p *eventPump) Unsubscribe(ch chan<- TopologyEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, listener := range p.listeners {
		if listener == ch {
			p.listeners = append(p.listeners[:i], p.listeners[i+1:]...)
			break
		}
	}
}

// Publish sends an event to all listeners
func (p *eventPump) Publish(event TopologyEvent) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return
	}

	// Buffer the event
	p.buffer.Push(event)

	// Send to all listeners (non-blocking)
	listeners := make([]chan<- TopologyEvent, len(p.listeners))
	copy(listeners, p.listeners)
	p.mu.RUnlock()

	for _, ch := range listeners {
		select {
		case ch <- event:
		default:
			// Channel full, skip
		}
	}
}

// Close shuts down the event pump
func (p *eventPump) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	p.closed = true
	p.listeners = nil
	p.buffer.Clear()
}