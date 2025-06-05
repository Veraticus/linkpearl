//go:build integration
// +build integration

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCLICommands tests the linkpearl CLI subcommands.
func TestCLICommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping CLI integration test in short mode")
	}

	// Build the binary
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "linkpearl")
	
	cmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/linkpearl")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build linkpearl: %v\nOutput: %s", err, output)
	}

	// Set up environment
	socketPath := filepath.Join(tmpDir, "linkpearl.sock")

	// Test version command
	t.Run("version", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "version")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Version command failed: %v\nOutput: %s", err, output)
		}
		
		if !strings.Contains(string(output), "linkpearl version") {
			t.Errorf("Version output doesn't contain expected text: %s", output)
		}
	})

	// Test copy/paste without daemon (should fail gracefully)
	t.Run("copy_without_daemon", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "copy", "--socket", socketPath)
		cmd.Stdin = strings.NewReader("test content")
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Error("Expected copy to fail without daemon running")
		}
		
		if !strings.Contains(string(output), "daemon not running") {
			t.Errorf("Expected daemon not running error, got: %s", output)
		}
	})

	// Test paste without daemon
	t.Run("paste_without_daemon", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "paste", "--socket", socketPath)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Error("Expected paste to fail without daemon running")
		}
		
		if !strings.Contains(string(output), "daemon not running") {
			t.Errorf("Expected daemon not running error, got: %s", output)
		}
	})

	// Test status without daemon
	t.Run("status_without_daemon", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "status", "--socket", socketPath)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Error("Expected status to fail without daemon running")
		}
		
		if !strings.Contains(string(output), "daemon not running") {
			t.Errorf("Expected daemon not running error, got: %s", output)
		}
	})

	// Start daemon and test commands
	t.Run("with_daemon", func(t *testing.T) {
		// Skip if we can't access clipboard (e.g., in CI)
		if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
			t.Skip("Skipping daemon tests in CI environment")
		}
		
		// Check if clipboard tools are available
		hasClipboard := false
		for _, tool := range []string{"xsel", "xclip", "wl-copy"} {
			if _, err := exec.LookPath(tool); err == nil {
				hasClipboard = true
				break
			}
		}
		
		if !hasClipboard {
			t.Skip("Skipping daemon tests - no clipboard tool available")
		}

		// Create a minimal config for testing
		secret := "test-secret-" + fmt.Sprintf("%d", time.Now().UnixNano())
		
		// Start daemon as a full node (listening)
		daemonCmd := exec.Command(binaryPath, "run",
			"--socket", socketPath,
			"--secret", secret,
			"--listen", ":0", // Use port 0 to get a random available port
			"--verbose",
		)
		
		// Set up to capture output
		daemonCmd.Stdout = os.Stdout
		daemonCmd.Stderr = os.Stderr
		
		if err := daemonCmd.Start(); err != nil {
			t.Fatalf("Failed to start daemon: %v", err)
		}
		
		// Ensure daemon is killed on test completion
		defer func() {
			if daemonCmd.Process != nil {
				_ = daemonCmd.Process.Kill()
				_ = daemonCmd.Wait()
			}
		}()
		
		// Wait for daemon to start
		var ready bool
		for i := 0; i < 30; i++ {
			if _, err := os.Stat(socketPath); err == nil {
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		
		if !ready {
			t.Fatal("Daemon failed to start within timeout")
		}
		
		// Additional wait to ensure daemon is fully ready
		time.Sleep(500 * time.Millisecond)
		
		// Test status command
		t.Run("status", func(t *testing.T) {
			cmd := exec.Command(binaryPath, "status", "--socket", socketPath, "--json")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Status command failed: %v\nOutput: %s", err, output)
			}
			
			// Should contain JSON with node_id
			if !strings.Contains(string(output), "\"node_id\"") {
				t.Errorf("Status output doesn't contain expected JSON: %s", output)
			}
		})
		
		// Test copy/paste
		t.Run("copy_paste", func(t *testing.T) {
			testContent := "Hello, linkpearl!"
			
			// Copy
			copyCmd := exec.Command(binaryPath, "copy", "--socket", socketPath)
			copyCmd.Stdin = strings.NewReader(testContent)
			if output, err := copyCmd.CombinedOutput(); err != nil {
				t.Fatalf("Copy command failed: %v\nOutput: %s", err, output)
			}
			
			// Paste
			pasteCmd := exec.Command(binaryPath, "paste", "--socket", socketPath)
			output, err := pasteCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Paste command failed: %v\nOutput: %s", err, output)
			}
			
			if string(output) != testContent {
				t.Errorf("Paste output = %q, want %q", output, testContent)
			}
		})
		
		// Test large content
		t.Run("large_content", func(t *testing.T) {
			// Create 1MB of content
			largeContent := strings.Repeat("x", 1024*1024)
			
			// Copy
			copyCmd := exec.Command(binaryPath, "copy", "--socket", socketPath)
			copyCmd.Stdin = strings.NewReader(largeContent)
			if output, err := copyCmd.CombinedOutput(); err != nil {
				t.Fatalf("Copy large content failed: %v\nOutput: %s", err, output)
			}
			
			// Paste
			pasteCmd := exec.Command(binaryPath, "paste", "--socket", socketPath)
			output, err := pasteCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Paste large content failed: %v\nOutput: %s", err, output)
			}
			
			if len(output) != len(largeContent) {
				t.Errorf("Paste output length = %d, want %d", len(output), len(largeContent))
			}
		})
	})
}

// TestNeoVimIntegration tests the NeoVim integration behavior.
func TestNeoVimIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping NeoVim integration test in short mode")
	}

	// Build the binary
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "linkpearl")
	
	cmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/linkpearl")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build linkpearl: %v\nOutput: %s", err, output)
	}

	// Set up environment
	socketPath := filepath.Join(tmpDir, "nonexistent.sock")
	
	// Test that commands fail silently in NeoVim mode
	t.Run("neovim_silent_failure", func(t *testing.T) {
		// Set NeoVim environment variable
		env := append(os.Environ(), "LINKPEARL_NEOVIM=1")
		
		// Test copy
		copyCmd := exec.Command(binaryPath, "copy", "--socket", socketPath)
		copyCmd.Env = env
		copyCmd.Stdin = strings.NewReader("test")
		output, err := copyCmd.CombinedOutput()
		if err != nil {
			t.Errorf("Copy should not fail in NeoVim mode: %v\nOutput: %s", err, output)
		}
		
		// Test paste
		pasteCmd := exec.Command(binaryPath, "paste", "--socket", socketPath)
		pasteCmd.Env = env
		output, err = pasteCmd.CombinedOutput()
		if err != nil {
			t.Errorf("Paste should not fail in NeoVim mode: %v\nOutput: %s", err, output)
		}
		
		// Output should be empty
		if len(output) > 0 {
			t.Errorf("Expected empty output in NeoVim mode, got: %s", output)
		}
	})
}