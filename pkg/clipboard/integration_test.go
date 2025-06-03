//go:build integration
// +build integration

package clipboard

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestRealClipboard(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Clipboard integration tests only supported on macOS and Linux")
	}

	clip, err := NewPlatformClipboard()
	if err != nil {
		t.Fatalf("NewPlatformClipboard() error = %v", err)
	}

	t.Run("Read and Write", func(t *testing.T) {
		// Save original clipboard content
		original, _ := clip.Read()
		defer clip.Write(original) // Restore after test

		// Test writing
		testContent := "linkpearl test content " + time.Now().Format(time.RFC3339Nano)
		if err := clip.Write(testContent); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Small delay to ensure clipboard is updated
		time.Sleep(100 * time.Millisecond)

		// Test reading
		content, err := clip.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}

		if content != testContent {
			t.Errorf("Read() = %q, want %q", content, testContent)
		}
	})

	t.Run("Watch", func(t *testing.T) {
		// Save original clipboard content
		original, _ := clip.Read()
		defer clip.Write(original) // Restore after test

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ch := clip.Watch(ctx)

		// Give watch goroutine time to start
		time.Sleep(100 * time.Millisecond)

		// Change clipboard content
		testContent := "watch test " + time.Now().Format(time.RFC3339Nano)
		go func() {
			time.Sleep(500 * time.Millisecond)
			if err := clip.Write(testContent); err != nil {
				t.Errorf("Write() error = %v", err)
			}
		}()

		// Wait for change notification
		select {
		case content := <-ch:
			if content != testContent {
				t.Errorf("Watch received %q, want %q", content, testContent)
			}
		case <-ctx.Done():
			t.Fatal("Timeout waiting for clipboard change")
		}
	})

	t.Run("Multiple Changes", func(t *testing.T) {
		// Save original clipboard content
		original, _ := clip.Read()
		defer clip.Write(original) // Restore after test

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		ch := clip.Watch(ctx)

		// Give watch goroutine time to start
		time.Sleep(100 * time.Millisecond)

		// Make multiple changes
		changes := []string{
			"change 1 " + time.Now().Format(time.RFC3339Nano),
			"change 2 " + time.Now().Format(time.RFC3339Nano),
			"change 3 " + time.Now().Format(time.RFC3339Nano),
		}

		go func() {
			for i, content := range changes {
				time.Sleep(500 * time.Millisecond)
				if err := clip.Write(content); err != nil {
					t.Errorf("Write() error on change %d: %v", i+1, err)
				}
			}
		}()

		// Verify we receive all changes
		received := 0
		for received < len(changes) {
			select {
			case content := <-ch:
				found := false
				for _, expected := range changes {
					if content == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Received unexpected content: %q", content)
				}
				received++
			case <-ctx.Done():
				t.Fatalf("Timeout waiting for changes, received %d/%d", received, len(changes))
			}
		}
	})
}

// TestPlatformSpecific tests platform-specific behavior
func TestPlatformSpecific(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Platform-specific tests only for macOS and Linux")
	}

	t.Run("Empty Clipboard", func(t *testing.T) {
		clip, err := NewPlatformClipboard()
		if err != nil {
			t.Fatalf("NewPlatformClipboard() error = %v", err)
		}

		// Save original content
		original, _ := clip.Read()
		defer clip.Write(original)

		// Clear clipboard
		if err := clip.Write(""); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		// Read empty clipboard
		content, err := clip.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}

		if content != "" {
			t.Errorf("Read() = %q, want empty string", content)
		}
	})

	t.Run("Unicode Content", func(t *testing.T) {
		clip, err := NewPlatformClipboard()
		if err != nil {
			t.Fatalf("NewPlatformClipboard() error = %v", err)
		}

		// Save original content
		original, _ := clip.Read()
		defer clip.Write(original)

		// Test with unicode content
		unicodeContent := "Hello 世界! 🎉 Émojis work! 🚀"
		if err := clip.Write(unicodeContent); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		content, err := clip.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}

		if content != unicodeContent {
			t.Errorf("Read() = %q, want %q", content, unicodeContent)
		}
	})

	t.Run("Multiline Content", func(t *testing.T) {
		clip, err := NewPlatformClipboard()
		if err != nil {
			t.Fatalf("NewPlatformClipboard() error = %v", err)
		}

		// Save original content
		original, _ := clip.Read()
		defer clip.Write(original)

		// Test with multiline content
		multilineContent := "Line 1\nLine 2\nLine 3\n"
		if err := clip.Write(multilineContent); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		content, err := clip.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}

		if content != multilineContent {
			t.Errorf("Read() = %q, want %q", content, multilineContent)
		}
	})
}