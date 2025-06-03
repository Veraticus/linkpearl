package clipboard

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMockClipboard(t *testing.T) {
	t.Run("Read and Write", func(t *testing.T) {
		mock := NewMockClipboard()

		// Initial read should return empty string
		content, err := mock.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if content != "" {
			t.Errorf("Initial Read() = %q, want empty string", content)
		}

		// Write and read back
		testContent := "Hello, clipboard!"
		if err := mock.Write(testContent); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		content, err = mock.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if content != testContent {
			t.Errorf("Read() = %q, want %q", content, testContent)
		}
	})

	t.Run("Watch", func(t *testing.T) {
		mock := NewMockClipboard()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := mock.Watch(ctx)

		// Verify watcher is registered
		if count := mock.GetWatcherCount(); count != 1 {
			t.Errorf("GetWatcherCount() = %d, want 1", count)
		}

		// Write should trigger watch
		testContent := "test content"
		go func() {
			time.Sleep(50 * time.Millisecond)
			mock.Write(testContent)
		}()

		select {
		case content := <-ch:
			if content != testContent {
				t.Errorf("Watch received %q, want %q", content, testContent)
			}
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for clipboard change")
		}

		// Cancel should clean up watcher
		cancel()
		time.Sleep(50 * time.Millisecond)

		if count := mock.GetWatcherCount(); count != 0 {
			t.Errorf("GetWatcherCount() after cancel = %d, want 0", count)
		}
	})

	t.Run("Multiple Watchers", func(t *testing.T) {
		mock := NewMockClipboard()
		ctx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		ch1 := mock.Watch(ctx1)
		ch2 := mock.Watch(ctx2)

		if count := mock.GetWatcherCount(); count != 2 {
			t.Errorf("GetWatcherCount() = %d, want 2", count)
		}

		// Both watchers should receive the change
		testContent := "broadcast test"
		mock.Write(testContent)

		for i, ch := range []<-chan string{ch1, ch2} {
			select {
			case content := <-ch:
				if content != testContent {
					t.Errorf("Watcher %d received %q, want %q", i+1, content, testContent)
				}
			case <-time.After(time.Second):
				t.Fatalf("Timeout waiting for watcher %d", i+1)
			}
		}
	})

	t.Run("EmitChange", func(t *testing.T) {
		mock := NewMockClipboard()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := mock.Watch(ctx)

		// EmitChange should update content and notify watchers
		testContent := "emitted content"
		go func() {
			time.Sleep(50 * time.Millisecond)
			mock.EmitChange(testContent)
		}()

		select {
		case content := <-ch:
			if content != testContent {
				t.Errorf("Watch received %q, want %q", content, testContent)
			}
		case <-time.After(time.Second):
			t.Fatal("Timeout waiting for emitted change")
		}

		// Verify content was updated
		content, err := mock.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if content != testContent {
			t.Errorf("Read() = %q, want %q", content, testContent)
		}
	})
}

func TestNewPlatformClipboard(t *testing.T) {
	// This test verifies platform clipboard creation doesn't panic
	// Actual functionality tests are in integration tests
	clip, err := NewPlatformClipboard()

	// On unsupported platforms, we expect an error
	if err == ErrNotSupported {
		t.Skip("Platform not supported")
	}

	// If clipboard tools aren't available (common in CI), skip
	if err != nil && strings.Contains(err.Error(), "no clipboard tool found") {
		t.Skipf("Clipboard tools not available: %v", err)
	}

	if err != nil {
		t.Fatalf("NewPlatformClipboard() error = %v", err)
	}

	if clip == nil {
		t.Fatal("NewPlatformClipboard() returned nil clipboard")
	}
}
