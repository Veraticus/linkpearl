package clipboard

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNoopClipboard(t *testing.T) {
	t.Run("basic read/write", func(t *testing.T) {
		clip := NewNoopClipboard()

		// Initial state should be empty
		content, err := clip.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if content != "" {
			t.Errorf("Initial content = %q, want empty", content)
		}

		// Write content
		testContent := "Hello, World!"
		if writeErr := clip.Write(testContent); writeErr != nil {
			t.Fatalf("Write() error = %v", writeErr)
		}

		// Read should return written content
		content, err = clip.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if content != testContent {
			t.Errorf("Read() = %q, want %q", content, testContent)
		}
	})

	t.Run("state tracking", func(t *testing.T) {
		clip := NewNoopClipboard()

		// Initial state
		state := clip.GetState()
		if state.SequenceNumber != 0 {
			t.Errorf("Initial SequenceNumber = %d, want 0", state.SequenceNumber)
		}
		if state.ContentHash != hashContent("") {
			t.Errorf("Initial ContentHash = %s, want hash of empty string", state.ContentHash)
		}

		// Write should update state
		content1 := "First content"
		if err := clip.Write(content1); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		state = clip.GetState()
		if state.SequenceNumber != 1 {
			t.Errorf("After write SequenceNumber = %d, want 1", state.SequenceNumber)
		}
		if state.ContentHash != hashContent(content1) {
			t.Errorf("After write ContentHash = %s, want hash of %q", state.ContentHash, content1)
		}

		// Another write should increment sequence
		content2 := "Second content"
		if err := clip.Write(content2); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		state = clip.GetState()
		if state.SequenceNumber != 2 {
			t.Errorf("After second write SequenceNumber = %d, want 2", state.SequenceNumber)
		}
		if state.ContentHash != hashContent(content2) {
			t.Errorf("After second write ContentHash = %s, want hash of %q", state.ContentHash, content2)
		}
	})

	t.Run("watch notifications", func(t *testing.T) {
		clip := NewNoopClipboard()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := clip.Watch(ctx)

		// Write should trigger notification
		if err := clip.Write("test content"); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		select {
		case <-ch:
			// Good, received notification
		case <-time.After(100 * time.Millisecond):
			t.Error("No notification received after write")
		}

		// Multiple writes should trigger multiple notifications
		for i := 0; i < 3; i++ {
			if err := clip.Write(string(rune('A' + i))); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
		}

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

		if notificationCount < 1 {
			t.Error("Expected at least one notification for multiple writes")
		}
	})

	t.Run("multiple watchers", func(t *testing.T) {
		clip := NewNoopClipboard()
		ctx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		ch1 := clip.Watch(ctx1)
		ch2 := clip.Watch(ctx2)

		// Write should notify all watchers
		if err := clip.Write("broadcast test"); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Both watchers should receive notification
		select {
		case <-ch1:
			// Good
		case <-time.After(100 * time.Millisecond):
			t.Error("Watcher 1 did not receive notification")
		}

		select {
		case <-ch2:
			// Good
		case <-time.After(100 * time.Millisecond):
			t.Error("Watcher 2 did not receive notification")
		}

		// Cancel one watcher
		cancel1()
		time.Sleep(50 * time.Millisecond) // Give time for cleanup

		// Drain any remaining notifications from ch1
	drainLoop:
		for {
			select {
			case _, ok := <-ch1:
				if !ok {
					// Channel is closed, stop draining
					break drainLoop
				}
				// Drain buffered notifications
			default:
				break drainLoop
			}
		}

		// Write again
		if err := clip.Write("second broadcast"); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Canceled watcher should not receive new notifications
		select {
		case _, ok := <-ch1:
			if ok {
				t.Error("Canceled watcher 1 should not receive notification")
			}
			// Channel closed is ok
		case <-time.After(50 * time.Millisecond):
			// Good, no notification
		}

		select {
		case <-ch2:
			// Good
		case <-time.After(100 * time.Millisecond):
			t.Error("Watcher 2 did not receive notification")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		clip := NewNoopClipboard()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start watchers
		var watchers []<-chan struct{}
		for i := 0; i < 5; i++ {
			watchers = append(watchers, clip.Watch(ctx))
		}

		// Concurrent writes and reads
		var wg sync.WaitGroup
		errors := make(chan error, 20)

		// Writers
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				content := string(rune('A' + id))
				if err := clip.Write(content); err != nil {
					errors <- err
				}
			}(i)
		}

		// Readers
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := clip.Read(); err != nil {
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Concurrent operation error: %v", err)
		}

		// All watchers should have received notifications
		for i, ch := range watchers {
			select {
			case <-ch:
				// Good
			default:
				t.Errorf("Watcher %d did not receive any notification", i)
			}
		}
	})

	t.Run("watcher cleanup", func(t *testing.T) {
		clip := NewNoopClipboard()

		// Create and cancel multiple watchers
		for i := 0; i < 10; i++ {
			ctx, cancel := context.WithCancel(context.Background())
			_ = clip.Watch(ctx)
			cancel()
		}

		// Give time for cleanup
		time.Sleep(100 * time.Millisecond)

		// Verify no watchers remain
		clip.watchMu.Lock()
		watcherCount := len(clip.watchChannels)
		clip.watchMu.Unlock()

		if watcherCount != 0 {
			t.Errorf("Expected 0 watchers after cleanup, got %d", watcherCount)
		}
	})

	t.Run("buffered channel behavior", func(t *testing.T) {
		clip := NewNoopClipboard()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := clip.Watch(ctx)

		// Write more than buffer size without consuming
		for i := 0; i < 20; i++ {
			if err := clip.Write(string(rune('A' + i))); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
		}

		// Should be able to read up to buffer size
		notificationCount := 0
		for i := 0; i < 10; i++ {
			select {
			case <-ch:
				notificationCount++
			default:
				// No more notifications
			}
		}

		// Should have received exactly buffer size (10)
		if notificationCount != 10 {
			t.Errorf("Expected 10 notifications (buffer size), got %d", notificationCount)
		}
	})
}

// TestNoopClipboardImplementsInterface ensures NoopClipboard implements Clipboard interface.
func TestNoopClipboardImplementsInterface(_ *testing.T) {
	var _ Clipboard = (*NoopClipboard)(nil)
	var _ Clipboard = NewNoopClipboard()
}
