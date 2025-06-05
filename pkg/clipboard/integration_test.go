//go:build integration
// +build integration

package clipboard

import (
	"bytes"
	"runtime"
	"testing"
	"time"
)

func TestClipboardWithTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping clipboard tests on Windows")
	}

	clip, err := NewPlatformClipboard()
	if err != nil {
		t.Skipf("skipping test: %v", err)
	}

	// Test that normal operations complete within timeout
	testContent := "Test timeout handling"

	start := time.Now()
	err = clip.Write(testContent)
	if err != nil {
		t.Errorf("write failed: %v", err)
	}
	writeTime := time.Since(start)

	if writeTime > 5*time.Second {
		t.Errorf("write took too long: %v", writeTime)
	}

	start = time.Now()
	content, err := clip.Read()
	if err != nil {
		t.Errorf("read failed: %v", err)
	}
	readTime := time.Since(start)

	if readTime > 5*time.Second {
		t.Errorf("read took too long: %v", readTime)
	}

	if content != testContent {
		t.Errorf("content mismatch: got %q, want %q", content, testContent)
	}
}

func TestClipboardSizeLimits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping clipboard tests on Windows")
	}

	clip, err := NewPlatformClipboard()
	if err != nil {
		t.Skipf("skipping test: %v", err)
	}

	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{
			name:    "small content",
			size:    100,
			wantErr: false,
		},
		{
			name:    "medium content",
			size:    1024 * 1024, // 1MB
			wantErr: false,
		},
		{
			name:    "large content near limit",
			size:    MaxClipboardSize - 1000,
			wantErr: false,
		},
		{
			name:    "content exceeds limit",
			size:    MaxClipboardSize + 1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate content of specified size
			content := string(bytes.Repeat([]byte("x"), tt.size))

			// Test write
			err := clip.Write(content)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected write error but got none")
				} else if !containsString(err.Error(), "ErrContentTooLarge") &&
					!containsString(err.Error(), "exceeds limit") {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected write error: %v", err)
				}

				// Verify we can read it back
				readContent, err := clip.Read()
				if err != nil {
					t.Errorf("failed to read back content: %v", err)
				} else if len(readContent) != tt.size {
					t.Errorf("content size mismatch: got %d, want %d",
						len(readContent), tt.size)
				}
			}
		})
	}
}

func TestClipboardInvalidUTF8(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping clipboard tests on Windows")
	}

	clip, err := NewPlatformClipboard()
	if err != nil {
		t.Skipf("skipping test: %v", err)
	}

	// Test writing invalid UTF-8
	invalidUTF8 := string([]byte{0xff, 0xfe, 0xfd})
	err = clip.Write(invalidUTF8)
	if err == nil {
		t.Error("expected error writing invalid UTF-8")
	} else if !containsString(err.Error(), "invalid UTF-8") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClipboardConcurrency(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping clipboard tests on Windows")
	}

	clip, err := NewPlatformClipboard()
	if err != nil {
		t.Skipf("skipping test: %v", err)
	}

	// Test concurrent reads and writes
	done := make(chan bool)
	errors := make(chan error, 10)

	// Writer goroutine
	go func() {
		for i := 0; i < 5; i++ {
			content := string(bytes.Repeat([]byte("W"), 1000))
			if err := clip.Write(content); err != nil {
				errors <- err
			}
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Reader goroutines
	for i := 0; i < 3; i++ {
		go func() {
			for j := 0; j < 5; j++ {
				if _, err := clip.Read(); err != nil {
					errors <- err
				}
				time.Sleep(5 * time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for completion
	for i := 0; i < 4; i++ {
		<-done
	}

	close(errors)

	// Check for errors
	var errCount int
	for err := range errors {
		t.Errorf("concurrent operation error: %v", err)
		errCount++
		if errCount > 5 {
			t.Fatal("too many errors in concurrent test")
		}
	}
}
