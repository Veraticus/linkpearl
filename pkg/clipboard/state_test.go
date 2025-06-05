package clipboard

import (
	"context"
	"testing"
	"time"
)

// TestClipboardStateTracking tests the state tracking functionality.
func TestClipboardStateTracking(t *testing.T) {
	t.Run("MockClipboard state tracking", func(t *testing.T) {
		mock := NewMockClipboard()

		// Initial state
		state := mock.GetState()
		if state.SequenceNumber != 0 {
			t.Errorf("Initial SequenceNumber = %d, want 0", state.SequenceNumber)
		}
		if state.ContentHash != hashContent("") {
			t.Errorf("Initial ContentHash = %s, want hash of empty string", state.ContentHash)
		}
		if state.LastModified.IsZero() {
			t.Error("Initial LastModified should not be zero")
		}

		// Write should update state
		initialTime := state.LastModified
		time.Sleep(10 * time.Millisecond) // Ensure time difference

		content1 := "Hello, World!"
		if err := mock.Write(content1); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		state = mock.GetState()
		if state.SequenceNumber != 1 {
			t.Errorf("After write SequenceNumber = %d, want 1", state.SequenceNumber)
		}
		if state.ContentHash != hashContent(content1) {
			t.Errorf("After write ContentHash = %s, want hash of %q", state.ContentHash, content1)
		}
		if !state.LastModified.After(initialTime) {
			t.Error("LastModified should be updated after write")
		}

		// Another write should increment sequence
		content2 := "New content"
		if err := mock.Write(content2); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		state = mock.GetState()
		if state.SequenceNumber != 2 {
			t.Errorf("After second write SequenceNumber = %d, want 2", state.SequenceNumber)
		}
		if state.ContentHash != hashContent(content2) {
			t.Errorf("After second write ContentHash = %s, want hash of %q", state.ContentHash, content2)
		}
	})

	t.Run("EmitChange updates state", func(t *testing.T) {
		mock := NewMockClipboard()

		// EmitChange should also update state
		content := "Emitted content"
		mock.EmitChange(content)

		state := mock.GetState()
		if state.SequenceNumber != 1 {
			t.Errorf("After EmitChange SequenceNumber = %d, want 1", state.SequenceNumber)
		}
		if state.ContentHash != hashContent(content) {
			t.Errorf("After EmitChange ContentHash = %s, want hash of %q", state.ContentHash, content)
		}
	})
}

// TestStatBasedNotifications tests the notification-only Watch behavior.
func TestStateBasedNotifications(t *testing.T) {
	t.Run("Watch returns notifications only", func(t *testing.T) {
		mock := NewMockClipboard()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := mock.Watch(ctx)

		// Write multiple times rapidly
		contents := []string{"First", "Second", "Third"}
		for _, content := range contents {
			if err := mock.Write(content); err != nil {
				t.Fatalf("Write(%q) error = %v", content, err)
			}
		}

		// Should receive notifications (might be less than 3 due to buffering)
		notificationCount := 0
		deadline := time.After(100 * time.Millisecond)

	loop:
		for {
			select {
			case <-ch:
				notificationCount++
			case <-deadline:
				break loop
			}
		}

		if notificationCount == 0 {
			t.Error("Expected at least one notification")
		}

		// Final read should get the last content
		content, err := mock.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if content != "Third" {
			t.Errorf("Read() = %q, want %q", content, "Third")
		}
	})

	t.Run("Buffered channel prevents blocking", func(t *testing.T) {
		mock := NewMockClipboard()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := mock.Watch(ctx)

		// Write more than buffer size without consuming
		// This should not block
		done := make(chan bool)
		go func() {
			for i := 0; i < 20; i++ {
				mock.EmitChange(string(rune('A' + i)))
			}
			done <- true
		}()

		select {
		case <-done:
			// Good, writes completed without blocking
		case <-time.After(time.Second):
			t.Fatal("Writes blocked despite buffered channel")
		}

		// Drain some notifications
		drainCount := 0
	drainLoop:
		for i := 0; i < 10; i++ {
			select {
			case <-ch:
				drainCount++
			default:
				break drainLoop
			}
		}

		// Due to buffer size of 10, we should get exactly 10
		if drainCount != 10 {
			t.Errorf("Drained %d notifications, expected 10 (buffer size)", drainCount)
		}
	})
}

// TestSequenceNumberMonotonic tests that sequence numbers always increase.
func TestSequenceNumberMonotonic(t *testing.T) {
	mock := NewMockClipboard()

	var lastSeq uint64

	// Mix of Write and EmitChange operations
	operations := []struct {
		name    string
		content string
		fn      func(string) error
	}{
		{"Write", "Content 1", func(c string) error { return mock.Write(c) }},
		{"EmitChange", "Content 2", func(c string) error { mock.EmitChange(c); return nil }},
		{"Write", "Content 3", func(c string) error { return mock.Write(c) }},
		{"EmitChange", "Content 4", func(c string) error { mock.EmitChange(c); return nil }},
	}

	for _, op := range operations {
		if err := op.fn(op.content); err != nil {
			t.Fatalf("%s(%q) error = %v", op.name, op.content, err)
		}

		state := mock.GetState()
		if state.SequenceNumber <= lastSeq {
			t.Errorf("After %s: SequenceNumber %d not greater than previous %d",
				op.name, state.SequenceNumber, lastSeq)
		}
		lastSeq = state.SequenceNumber
	}

	if lastSeq != 4 {
		t.Errorf("Final sequence number = %d, want 4", lastSeq)
	}
}

// TestPullBasedSynchronization demonstrates the pull-based pattern.
func TestPullBasedSynchronization(t *testing.T) {
	mock := NewMockClipboard()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notifications := mock.Watch(ctx)

	// Simulate sync engine behavior
	receivedContents := make([]string, 0)

	// Producer: rapid clipboard changes
	go func() {
		for i := 0; i < 5; i++ {
			content := string(rune('A' + i))
			mock.EmitChange(content)
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Consumer: pull-based processing
	deadline := time.After(100 * time.Millisecond)
	lastState := mock.GetState()

loop:
	for {
		select {
		case <-notifications:
			// Notification received - check if state actually changed
			newState := mock.GetState()
			if newState.SequenceNumber > lastState.SequenceNumber {
				// State changed, pull content
				content, err := mock.Read()
				if err == nil {
					receivedContents = append(receivedContents, content)
					lastState = newState
				}
			}
		case <-deadline:
			break loop
		}
	}

	// We should have received some but maybe not all due to coalescing
	if len(receivedContents) == 0 {
		t.Error("No contents received")
	}

	// Final content should be 'E'
	finalContent, _ := mock.Read()
	if finalContent != "E" {
		t.Errorf("Final content = %q, want 'E'", finalContent)
	}

	// Final state sequence should be 5
	finalState := mock.GetState()
	if finalState.SequenceNumber != 5 {
		t.Errorf("Final SequenceNumber = %d, want 5", finalState.SequenceNumber)
	}
}

// TestStateConsistency verifies state remains consistent.
func TestStateConsistency(t *testing.T) {
	mock := NewMockClipboard()

	// Write content
	content := "Test content"
	if err := mock.Write(content); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// State hash should match content hash
	state := mock.GetState()
	expectedHash := hashContent(content)
	if state.ContentHash != expectedHash {
		t.Errorf("State ContentHash = %s, want %s", state.ContentHash, expectedHash)
	}

	// Read content should match
	readContent, err := mock.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if readContent != content {
		t.Errorf("Read() = %q, want %q", readContent, content)
	}

	// Hash of read content should match state
	if hashContent(readContent) != state.ContentHash {
		t.Error("Content hash mismatch between Read() and GetState()")
	}
}
