//go:build darwin
// +build darwin

package clipboard

import (
	"os/exec"
	"strings"
	"testing"
)

// TestPbcopyPbpaste tests if pbcopy and pbpaste work in the current environment
func TestPbcopyPbpaste(t *testing.T) {
	// Test 1: Check if pbcopy exists
	if _, err := exec.LookPath("pbcopy"); err != nil {
		t.Errorf("pbcopy not found: %v", err)
	}

	// Test 2: Check if pbpaste exists
	if _, err := exec.LookPath("pbpaste"); err != nil {
		t.Errorf("pbpaste not found: %v", err)
	}

	// Test 3: Try a simple write/read cycle
	testContent := "Hello, World!"
	
	// Write using pbcopy
	cmd := exec.Command("pbcopy")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to create stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start pbcopy: %v", err)
	}

	if _, err := stdin.Write([]byte(testContent)); err != nil {
		t.Fatalf("Failed to write to pbcopy: %v", err)
	}

	stdin.Close()

	if err := cmd.Wait(); err != nil {
		t.Fatalf("pbcopy failed: %v", err)
	}

	// Read using pbpaste
	output, err := exec.Command("pbpaste").Output()
	if err != nil {
		t.Fatalf("pbpaste failed: %v", err)
	}

	result := string(output)
	if result != testContent {
		t.Errorf("Content mismatch: wrote %q, read %q", testContent, result)
	}

	t.Logf("pbcopy/pbpaste test successful: wrote and read %q", result)
}

// TestDarwinClipboardBasic tests basic Darwin clipboard operations
func TestDarwinClipboardBasic(t *testing.T) {
	clip, err := newPlatformClipboard()
	if err != nil {
		t.Fatalf("Failed to create clipboard: %v", err)
	}

	// Test write
	testContent := "Test content"
	if err := clip.Write(testContent); err != nil {
		t.Fatalf("Failed to write to clipboard: %v", err)
	}

	// Test read
	content, err := clip.Read()
	if err != nil {
		t.Fatalf("Failed to read from clipboard: %v", err)
	}

	if content != testContent {
		t.Errorf("Content mismatch: wrote %q, read %q", testContent, content)
	}
}

// TestDarwinClipboardChangeCount tests the change count mechanism
func TestDarwinClipboardChangeCount(t *testing.T) {
	clip, ok := (&DarwinClipboard{}).(*DarwinClipboard)
	if !ok {
		t.Skip("Not a Darwin clipboard")
	}

	// Get initial change count
	count1 := clip.getChangeCount()
	t.Logf("Initial change count: %d", count1)

	// Write something
	if err := clip.Write("Test"); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Get new change count
	count2 := clip.getChangeCount()
	t.Logf("Change count after write: %d", count2)

	// Check if AppleScript is working
	if count1 == -1 && count2 == -1 {
		t.Log("WARNING: AppleScript change count detection not working (both counts are -1)")
		
		// Try to run AppleScript directly
		cmd := exec.Command("osascript", "-e", "return 1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("AppleScript test failed: %v, output: %s", err, output)
		} else {
			t.Logf("AppleScript basic test passed, output: %s", strings.TrimSpace(string(output)))
		}
	}
}