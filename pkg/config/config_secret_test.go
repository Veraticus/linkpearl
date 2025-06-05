package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretFileHandling(t *testing.T) {
	t.Run("secret from file", func(t *testing.T) {
		// Create temp file with secret
		tmpDir := t.TempDir()
		secretFile := filepath.Join(tmpDir, "secret.txt")
		secretContent := "test-secret-from-file"

		if err := os.WriteFile(secretFile, []byte(secretContent), 0600); err != nil {
			t.Fatalf("failed to write secret file: %v", err)
		}

		cfg := NewConfig()
		cfg.SecretFile = secretFile
		cfg.NodeID = "test-node"
		cfg.Join = []string{"localhost:9437"}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("validation failed: %v", err)
		}

		if cfg.Secret != secretContent {
			t.Errorf("expected secret %q, got %q", secretContent, cfg.Secret)
		}
	})

	t.Run("secret from file with whitespace", func(t *testing.T) {
		// Create temp file with secret containing whitespace
		tmpDir := t.TempDir()
		secretFile := filepath.Join(tmpDir, "secret.txt")
		secretWithSpaces := "  test-secret-with-spaces  \n"
		expectedSecret := "test-secret-with-spaces"

		if err := os.WriteFile(secretFile, []byte(secretWithSpaces), 0600); err != nil {
			t.Fatalf("failed to write secret file: %v", err)
		}

		cfg := NewConfig()
		cfg.SecretFile = secretFile
		cfg.NodeID = "test-node"
		cfg.Join = []string{"localhost:9437"}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("validation failed: %v", err)
		}

		if cfg.Secret != expectedSecret {
			t.Errorf("expected secret %q, got %q", expectedSecret, cfg.Secret)
		}
	})

	t.Run("both secret and secret-file specified", func(t *testing.T) {
		tmpDir := t.TempDir()
		secretFile := filepath.Join(tmpDir, "secret.txt")

		if err := os.WriteFile(secretFile, []byte("file-secret"), 0600); err != nil {
			t.Fatalf("failed to write secret file: %v", err)
		}

		cfg := NewConfig()
		cfg.Secret = "direct-secret"
		cfg.SecretFile = secretFile
		cfg.NodeID = "test-node"
		cfg.Join = []string{"localhost:9437"}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error when both secret and secret-file are specified")
		}

		if err.Error() != "cannot specify both --secret and --secret-file" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing secret file", func(t *testing.T) {
		cfg := NewConfig()
		cfg.SecretFile = "/non/existent/file.txt"
		cfg.NodeID = "test-node"
		cfg.Join = []string{"localhost:9437"}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error when secret file doesn't exist")
		}

		// Check if the error contains "no such file"
		if !strings.Contains(err.Error(), "no such file") {
			t.Errorf("expected file not found error, got: %v", err)
		}
	})

	t.Run("no secret or secret-file", func(t *testing.T) {
		cfg := NewConfig()
		cfg.NodeID = "test-node"
		cfg.Join = []string{"localhost:9437"}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error when neither secret nor secret-file is specified")
		}

		if err.Error() != "secret is required (use --secret, LINKPEARL_SECRET, or --secret-file)" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("secret file from environment", func(t *testing.T) {
		// Create temp file with secret
		tmpDir := t.TempDir()
		secretFile := filepath.Join(tmpDir, "secret.txt")
		secretContent := "env-secret-from-file"

		if err := os.WriteFile(secretFile, []byte(secretContent), 0600); err != nil {
			t.Fatalf("failed to write secret file: %v", err)
		}

		// Set environment variable
		if err := os.Setenv("LINKPEARL_SECRET_FILE", secretFile); err != nil {
			t.Fatalf("failed to set environment variable: %v", err)
		}
		defer func() {
			if err := os.Unsetenv("LINKPEARL_SECRET_FILE"); err != nil {
				t.Errorf("failed to unset environment variable: %v", err)
			}
		}()

		cfg := NewConfig()
		cfg.NodeID = "test-node"
		cfg.Join = []string{"localhost:9437"}
		cfg.LoadFromEnv()

		if cfg.SecretFile != secretFile {
			t.Errorf("expected secret file %q, got %q", secretFile, cfg.SecretFile)
		}

		if err := cfg.Validate(); err != nil {
			t.Fatalf("validation failed: %v", err)
		}

		if cfg.Secret != secretContent {
			t.Errorf("expected secret %q, got %q", secretContent, cfg.Secret)
		}
	})

	t.Run("unreadable secret file", func(t *testing.T) {
		// Skip on Windows as file permissions work differently
		if os.Getenv("GOOS") == "windows" {
			t.Skip("skipping permission test on Windows")
		}

		tmpDir := t.TempDir()
		secretFile := filepath.Join(tmpDir, "secret.txt")

		if err := os.WriteFile(secretFile, []byte("secret"), 0600); err != nil {
			t.Fatalf("failed to write secret file: %v", err)
		}

		// Make file unreadable
		if err := os.Chmod(secretFile, 0000); err != nil {
			t.Fatalf("failed to chmod file: %v", err)
		}

		// Ensure we can chmod it back in cleanup
		defer func() {
			if err := os.Chmod(secretFile, 0600); err != nil {
				t.Errorf("failed to restore file permissions: %v", err)
			}
		}()

		cfg := NewConfig()
		cfg.SecretFile = secretFile
		cfg.NodeID = "test-node"
		cfg.Join = []string{"localhost:9437"}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error when secret file is unreadable")
		}

		// Check if the error contains "permission denied"
		if !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("expected permission error, got: %v", err)
		}
	})
}
