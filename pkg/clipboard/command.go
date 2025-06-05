package clipboard

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// CommandTimeout is the maximum time allowed for clipboard operations.
const CommandTimeout = 5 * time.Second

// CommandConfig holds configuration for command execution.
type CommandConfig struct {
	// Timeout for command execution (default: CommandTimeout)
	Timeout time.Duration

	// MaxOutputSize limits the amount of data read (default: MaxClipboardSize)
	MaxOutputSize int

	// Logger for debugging command execution
	Logger func(format string, args ...interface{})
}

// DefaultCommandConfig returns config with production-ready defaults.
func DefaultCommandConfig() *CommandConfig {
	return &CommandConfig{
		Timeout:       CommandTimeout,
		MaxOutputSize: MaxClipboardSize,
		Logger:        func(string, ...interface{}) {}, // no-op by default
	}
}

// RunCommand executes a command with proper timeout and resource management.
func RunCommand(name string, args []string, config *CommandConfig) ([]byte, error) {
	if config == nil {
		config = DefaultCommandConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	// Set up pipes before starting
	output, err := cmd.Output()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("command %s timed out after %v", name, config.Timeout)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Special handling for known exit codes
			if exitErr.ExitCode() == 1 && len(output) == 0 {
				// Empty clipboard on some systems
				return []byte{}, nil
			}
			return nil, fmt.Errorf("command %s failed with exit code %d: %w",
				name, exitErr.ExitCode(), err)
		}
		return nil, fmt.Errorf("command %s failed: %w", name, err)
	}

	if len(output) > config.MaxOutputSize {
		return nil, fmt.Errorf("command output exceeds maximum size of %d bytes",
			config.MaxOutputSize)
	}

	return output, nil
}

// RunCommandWithInput executes a command with stdin input.
func RunCommandWithInput(name string, args []string, input []byte, config *CommandConfig) error {
	if config == nil {
		config = DefaultCommandConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	// Set up stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Ensure cleanup happens regardless of error
	defer func() {
		if stdin != nil {
			_ = stdin.Close()
		}
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", name, err)
	}

	// Write input
	if _, err := stdin.Write(input); err != nil {
		return fmt.Errorf("failed to write to %s: %w", name, err)
	}

	// Close stdin to signal EOF
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("failed to close stdin: %w", err)
	}
	stdin = nil // Prevent double close in defer

	// Wait for completion
	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("command %s timed out after %v", name, config.Timeout)
		}
		return fmt.Errorf("%s failed: %w", name, err)
	}

	return nil
}
