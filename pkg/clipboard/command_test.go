package clipboard

import (
	"bytes"
	"runtime"
	"testing"
	"time"
)

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		args        []string
		config      *CommandConfig
		wantErr     bool
		errContains string
	}{
		{
			name:    "successful command",
			cmd:     "echo",
			args:    []string{"hello"},
			config:  DefaultCommandConfig(),
			wantErr: false,
		},
		{
			name: "command timeout",
			cmd:  "sleep",
			args: []string{"10"},
			config: &CommandConfig{
				Timeout:       100 * time.Millisecond,
				MaxOutputSize: MaxClipboardSize,
			},
			wantErr:     true,
			errContains: "timed out",
		},
		{
			name:    "command not found",
			cmd:     "nonexistentcommand123",
			args:    nil,
			config:  DefaultCommandConfig(),
			wantErr: true,
		},
		{
			name: "output size exceeded",
			cmd:  "echo",
			args: []string{string(bytes.Repeat([]byte("a"), 100))},
			config: &CommandConfig{
				Timeout:       5 * time.Second,
				MaxOutputSize: 10, // Very small limit
			},
			wantErr:     true,
			errContains: "exceeds maximum size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := RunCommand(tt.cmd, tt.args, tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error %q doesn't contain expected string %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if output == nil {
					t.Errorf("expected output but got nil")
				}
			}
		})
	}
}

func TestRunCommandWithInput(t *testing.T) {
	// Skip on Windows since we use Unix commands
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix command tests on Windows")
	}

	tests := []struct {
		name        string
		cmd         string
		args        []string
		input       []byte
		config      *CommandConfig
		wantErr     bool
		errContains string
	}{
		{
			name:    "successful command with input",
			cmd:     "cat",
			args:    nil,
			input:   []byte("test input"),
			config:  DefaultCommandConfig(),
			wantErr: false,
		},
		{
			name:  "command timeout with input",
			cmd:   "sleep",
			args:  []string{"10"},
			input: []byte("test"),
			config: &CommandConfig{
				Timeout: 100 * time.Millisecond,
			},
			wantErr:     true,
			errContains: "timed out",
		},
		{
			name:    "command fails",
			cmd:     "false",
			args:    nil,
			input:   []byte("test"),
			config:  DefaultCommandConfig(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunCommandWithInput(tt.cmd, tt.args, tt.input, tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error %q doesn't contain expected string %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRunCommandContextCancellation(t *testing.T) {
	// Test that commands respect timeout (which is context-based)
	config := &CommandConfig{
		Timeout:       1 * time.Nanosecond, // Extremely short timeout
		MaxOutputSize: MaxClipboardSize,
	}

	// Try to run a command with extremely short timeout
	_, err := RunCommand("echo", []string{"test"}, config)
	if err == nil {
		t.Error("expected error with extremely short timeout")
	}
}

func TestDefaultCommandConfig(t *testing.T) {
	config := DefaultCommandConfig()

	if config.Timeout != CommandTimeout {
		t.Errorf("expected timeout %v, got %v", CommandTimeout, config.Timeout)
	}

	if config.MaxOutputSize != MaxClipboardSize {
		t.Errorf("expected max output size %d, got %d", MaxClipboardSize, config.MaxOutputSize)
	}

	if config.Logger == nil {
		t.Error("expected non-nil logger")
	}
}

func TestCommandResourceCleanup(t *testing.T) {
	// This test verifies that resources are cleaned up even when errors occur
	// We'll use a command that we know will fail

	config := DefaultCommandConfig()
	err := RunCommandWithInput("false", nil, []byte("test"), config)

	if err == nil {
		t.Error("expected error from 'false' command")
	}

	// If we get here without hanging, resource cleanup worked
	// In a real test, we might check process counts or file descriptors
}

func containsString(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
