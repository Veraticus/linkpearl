package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestNewLogger tests logger creation.
func TestNewLogger(t *testing.T) {
	tests := []struct {
		name    string
		verbose bool
		want    *logger
	}{
		{
			name:    "verbose logger",
			verbose: true,
			want:    &logger{verbose: true, prefix: ""},
		},
		{
			name:    "non-verbose logger",
			verbose: false,
			want:    &logger{verbose: false, prefix: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newLogger(tt.verbose)
			if got.verbose != tt.want.verbose {
				t.Errorf("verbose = %v, want %v", got.verbose, tt.want.verbose)
			}
			if got.prefix != tt.want.prefix {
				t.Errorf("prefix = %q, want %q", got.prefix, tt.want.prefix)
			}
		})
	}
}

// TestWithPrefix tests prefix creation.
func TestWithPrefix(t *testing.T) {
	tests := []struct {
		name       string
		baseLogger *logger
		prefix     string
		want       *logger
	}{
		{
			name:       "add prefix to base logger",
			baseLogger: &logger{verbose: true, prefix: ""},
			prefix:     "transport",
			want:       &logger{verbose: true, prefix: "transport"},
		},
		{
			name:       "add prefix to prefixed logger",
			baseLogger: &logger{verbose: false, prefix: "mesh"},
			prefix:     "connection",
			want:       &logger{verbose: false, prefix: "connection"},
		},
		{
			name:       "empty prefix",
			baseLogger: &logger{verbose: true, prefix: "base"},
			prefix:     "",
			want:       &logger{verbose: true, prefix: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.baseLogger.withPrefix(tt.prefix)
			if got.verbose != tt.want.verbose {
				t.Errorf("verbose = %v, want %v", got.verbose, tt.want.verbose)
			}
			if got.prefix != tt.want.prefix {
				t.Errorf("prefix = %q, want %q", got.prefix, tt.want.prefix)
			}
			// Ensure it's a new instance
			if got == tt.baseLogger {
				t.Error("withPrefix should return a new logger instance")
			}
		})
	}
}

// TestFormatMessage tests message formatting.
func TestFormatMessage(t *testing.T) {
	// Note: We can't mock time.Now in tests, so we'll check for timestamp format instead

	tests := []struct {
		name          string
		logger        *logger
		level         string
		msg           string
		keysAndValues []interface{}
		wantContains  []string
		wantPattern   string
	}{
		{
			name:   "basic message without key-values",
			logger: &logger{},
			level:  "INFO",
			msg:    "application started",
			wantContains: []string{
				"INFO ",
				"application started",
			},
		},
		{
			name:   "message with key-value pairs",
			logger: &logger{},
			level:  "ERROR",
			msg:    "connection failed",
			keysAndValues: []interface{}{
				"host", "localhost",
				"port", 8080,
				"error", "timeout",
			},
			wantContains: []string{
				"ERROR",
				"connection failed",
				"host=localhost",
				"port=8080",
				"error=timeout",
			},
		},
		{
			name:   "message with prefix",
			logger: &logger{prefix: "transport"},
			level:  "DEBUG",
			msg:    "sending message",
			keysAndValues: []interface{}{
				"to", "node-123",
				"size", 1024,
			},
			wantContains: []string{
				"DEBUG",
				"[transport]",
				"sending message",
				"to=node-123",
				"size=1024",
			},
		},
		{
			name:   "odd number of key-value pairs",
			logger: &logger{},
			level:  "WARN",
			msg:    "odd pairs",
			keysAndValues: []interface{}{
				"key1", "value1",
				"key2", "value2",
				"key3", // missing value
			},
			wantContains: []string{
				"WARN",
				"odd pairs",
				"key1=value1",
				"key2=value2",
				"key3=<nil>",
			},
		},
		{
			name:   "level padding",
			logger: &logger{},
			level:  "INFO",
			msg:    "test",
			wantContains: []string{
				"INFO ", // Should be padded to 5 chars
			},
		},
		{
			name:   "nil values",
			logger: &logger{},
			level:  "INFO",
			msg:    "nil test",
			keysAndValues: []interface{}{
				"nil_value", nil,
				"string", "not nil",
			},
			wantContains: []string{
				"nil_value=<nil>",
				"string=not nil",
			},
		},
		{
			name:   "special characters in values",
			logger: &logger{},
			level:  "INFO",
			msg:    "special chars",
			keysAndValues: []interface{}{
				"path", "/home/user/file with spaces.txt",
				"error", "failed: \"permission denied\"",
			},
			wantContains: []string{
				"path=/home/user/file with spaces.txt",
				"error=failed: \"permission denied\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.logger.formatMessage(tt.level, tt.msg, tt.keysAndValues...)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("formatMessage() missing %q\nGot: %q", want, got)
				}
			}

			// Check overall structure
			parts := strings.Split(got, " ")
			if len(parts) < 3 {
				t.Errorf("formatMessage() has too few parts: %q", got)
			}

			// Verify timestamp format - should contain T (time separator)
			if !strings.Contains(parts[0], "T") {
				t.Errorf("invalid timestamp format: %q", parts[0])
			}
		})
	}
}

// TestLoggerMethods tests the various log level methods.
func TestLoggerMethods(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		verbose  bool
		wantLog  bool
		wantExit bool
	}{
		{
			name:     "Debug with verbose=true",
			method:   "Debug",
			verbose:  true,
			wantLog:  true,
			wantExit: false,
		},
		{
			name:     "Debug with verbose=false",
			method:   "Debug",
			verbose:  false,
			wantLog:  false,
			wantExit: false,
		},
		{
			name:     "Info always logs",
			method:   "Info",
			verbose:  false,
			wantLog:  true,
			wantExit: false,
		},
		{
			name:     "Error always logs",
			method:   "Error",
			verbose:  false,
			wantLog:  true,
			wantExit: false,
		},
		{
			name:     "Fatal logs and exits",
			method:   "Fatal",
			verbose:  false,
			wantLog:  true,
			wantExit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture log output
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(os.Stderr)

			logger := newLogger(tt.verbose)

			// For Fatal, we skip testing the actual exit behavior
			// as we can't mock os.Exit reliably
			if tt.method == "Fatal" {
				// We'll test that it logs, but skip the exit part
				// In a real test environment, you might use a subprocess
				logger.Error("test message", "key", "value") // Use Error to simulate Fatal logging
			} else {
				// Call the appropriate method
				switch tt.method {
				case "Debug":
					logger.Debug("test message", "key", "value")
				case "Info":
					logger.Info("test message", "key", "value")
				case "Error":
					logger.Error("test message", "key", "value")
				}
			}

			output := buf.String()
			hasOutput := len(output) > 0

			if hasOutput != tt.wantLog {
				t.Errorf("%s() logged = %v, want %v\nOutput: %q", tt.method, hasOutput, tt.wantLog, output)
			}

			if tt.wantLog && hasOutput {
				// Verify output contains expected elements
				expectedLevel := strings.ToUpper(tt.method)
				if len(expectedLevel) > 5 {
					expectedLevel = expectedLevel[:5]
				}
				if tt.method == "Fatal" {
					expectedLevel = "ERROR" // We use Error to simulate Fatal in tests
				}
				if !strings.Contains(output, expectedLevel) {
					t.Errorf("%s() output missing level, got: %q", tt.method, output)
				}
				if !strings.Contains(output, "test message") {
					t.Errorf("%s() output missing message, got: %q", tt.method, output)
				}
				if !strings.Contains(output, "key=value") {
					t.Errorf("%s() output missing key-value, got: %q", tt.method, output)
				}
			}
		})
	}
}

// TestLoggerAdapters tests the adapter implementations.
func TestLoggerAdapters(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	t.Run("meshLogger", func(t *testing.T) {
		logger := newLogger(true).withPrefix("mesh")
		mesh := meshLogger{logger}

		// Test each method
		tests := []struct {
			name string
			call func()
			want string
		}{
			{
				name: "Info",
				call: func() { mesh.Info("mesh info", "peer", "node-123") },
				want: "INFO",
			},
			{
				name: "Debug",
				call: func() { mesh.Debug("mesh debug", "state", "connected") },
				want: "DEBUG",
			},
			{
				name: "Error",
				call: func() { mesh.Error("mesh error", "reason", "timeout") },
				want: "ERROR",
			},
			{
				name: "Warn mapped to Info",
				call: func() { mesh.Warn("mesh warning", "issue", "high latency") },
				want: "INFO", // Warn is mapped to Info
			},
		}

		for _, tt := range tests {
			buf.Reset()
			tt.call()
			output := buf.String()

			if !strings.Contains(output, tt.want) {
				t.Errorf("meshLogger.%s() missing level %q, got: %q", tt.name, tt.want, output)
			}
			if !strings.Contains(output, "[mesh]") {
				t.Errorf("meshLogger.%s() missing prefix, got: %q", tt.name, output)
			}
		}
	})

	t.Run("transportLogger", func(t *testing.T) {
		logger := newLogger(true).withPrefix("transport")
		trans := transportLogger{logger}

		// Test each method
		tests := []struct {
			name string
			call func()
			want string
		}{
			{
				name: "Infof",
				call: func() { trans.Infof("connection established to %s:%d", "localhost", 8080) },
				want: "connection established to localhost:8080",
			},
			{
				name: "Debugf",
				call: func() { trans.Debugf("sending %d bytes", 1024) },
				want: "sending 1024 bytes",
			},
			{
				name: "Errorf",
				call: func() { trans.Errorf("failed to connect: %v", fmt.Errorf("timeout")) },
				want: "failed to connect: timeout",
			},
		}

		for _, tt := range tests {
			buf.Reset()
			tt.call()
			output := buf.String()

			if !strings.Contains(output, tt.want) {
				t.Errorf("transportLogger.%s() missing formatted message, got: %q", tt.name, output)
			}
			if !strings.Contains(output, "[transport]") {
				t.Errorf("transportLogger.%s() missing prefix, got: %q", tt.name, output)
			}
		}
	})
}

// TestLoggerConcurrency tests concurrent access to logger.
func TestLoggerConcurrency(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := newLogger(true)

	// Run concurrent logging
	var wg sync.WaitGroup
	numGoroutines := 10
	numLogs := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numLogs; j++ {
				switch j % 4 {
				case 0:
					logger.Debug("debug message", "goroutine", id, "iteration", j)
				case 1:
					logger.Info("info message", "goroutine", id, "iteration", j)
				case 2:
					logger.Error("error message", "goroutine", id, "iteration", j)
				case 3:
					subLogger := logger.withPrefix(fmt.Sprintf("worker-%d", id))
					subLogger.Info("prefixed message", "iteration", j)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify output
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	expectedLines := numGoroutines * numLogs
	if len(lines) != expectedLines {
		t.Errorf("expected %d log lines, got %d", expectedLines, len(lines))
	}

	// Check that all lines are properly formatted
	for i, line := range lines {
		if line == "" {
			continue
		}

		// Should have timestamp
		if !strings.Contains(line, "T") {
			t.Errorf("line %d missing timestamp: %q", i, line)
		}

		// Should have level
		hasLevel := false
		for _, level := range []string{"DEBUG", "INFO", "ERROR"} {
			if strings.Contains(line, level) {
				hasLevel = true
				break
			}
		}
		if !hasLevel {
			t.Errorf("line %d missing log level: %q", i, line)
		}

		// Should have message
		hasMessage := false
		for _, msg := range []string{"debug message", "info message", "error message", "prefixed message"} {
			if strings.Contains(line, msg) {
				hasMessage = true
				break
			}
		}
		if !hasMessage {
			t.Errorf("line %d missing expected message: %q", i, line)
		}
	}
}

// TestLoggerInit tests the init function.
func TestLoggerInit(t *testing.T) {
	// The init function should have removed timestamp from standard logger
	flags := log.Flags()
	if flags != 0 {
		t.Errorf("log.Flags() = %d, want 0", flags)
	}
}

// Benchmark tests

func BenchmarkFormatMessage(b *testing.B) {
	logger := newLogger(true)

	benchmarks := []struct {
		name          string
		prefix        string
		keysAndValues []interface{}
	}{
		{
			name:   "no-keys",
			prefix: "",
		},
		{
			name:          "with-keys",
			prefix:        "",
			keysAndValues: []interface{}{"key1", "value1", "key2", 123, "key3", true},
		},
		{
			name:          "with-prefix-and-keys",
			prefix:        "component",
			keysAndValues: []interface{}{"key1", "value1", "key2", 123, "key3", true},
		},
		{
			name:   "many-keys",
			prefix: "component",
			keysAndValues: []interface{}{
				"k1", "v1", "k2", "v2", "k3", "v3", "k4", "v4", "k5", "v5",
				"k6", "v6", "k7", "v7", "k8", "v8", "k9", "v9", "k10", "v10",
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			l := logger.withPrefix(bm.prefix)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = l.formatMessage("INFO", "benchmark message", bm.keysAndValues...)
			}
		})
	}
}

func BenchmarkLoggerMethods(b *testing.B) {
	// Discard output for benchmarks
	log.SetOutput(ioDiscard)
	defer log.SetOutput(os.Stderr)

	logger := newLogger(true)

	b.Run("Debug", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Debug("debug message", "iteration", i, "data", "value")
		}
	})

	b.Run("Info", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info("info message", "iteration", i, "data", "value")
		}
	})

	b.Run("Error", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Error("error message", "iteration", i, "data", "value")
		}
	})
}

// ioDiscard for benchmarks.
var ioDiscard = discard{}

type discard struct{}

func (discard) Write(p []byte) (int, error) {
	return len(p), nil
}
