// Package main - logger.go implements structured logging for the linkpearl application.
//
// # Overview
//
// This file provides a lightweight structured logging system that supports:
//   - Multiple log levels (DEBUG, INFO, ERROR, FATAL)
//   - Key-value pair formatting for structured data
//   - Component-specific prefixes for log categorization
//   - Adapter interfaces for compatibility with package-specific loggers
//
// # Log Format
//
// Logs are formatted with the following structure:
//
//	2024-01-15T10:30:45.123Z INFO  [prefix] message key1=value1 key2=value2
//
// Components:
//   - ISO 8601 timestamp with millisecond precision
//   - Fixed-width log level for easy parsing
//   - Optional component prefix in square brackets
//   - Human-readable message
//   - Structured key-value pairs for additional context
//
// # Log Levels
//
//   - DEBUG: Detailed information for troubleshooting (only shown in verbose mode)
//   - INFO: General informational messages about application state
//   - ERROR: Error conditions that don't prevent operation
//   - FATAL: Critical errors that cause application termination
//
// # Component Loggers
//
// The logger supports creating prefixed sub-loggers for different components:
//
//	mainLogger := newLogger(verbose)
//	transportLogger := mainLogger.withPrefix("transport")
//	meshLogger := mainLogger.withPrefix("mesh")
//
// This helps identify which component generated each log line, making debugging easier.
//
// # Adapter Pattern
//
// The file includes adapter types (meshLogger, transportLogger) that implement
// package-specific logger interfaces, allowing seamless integration with external
// packages that expect different logging interfaces while maintaining consistent
// formatting throughout the application.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// logger provides structured logging for the application
type logger struct {
	verbose bool
	prefix  string
}

// newLogger creates a new logger
func newLogger(verbose bool) *logger {
	return &logger{
		verbose: verbose,
		prefix:  "",
	}
}

// withPrefix creates a new logger with a prefix
func (l *logger) withPrefix(prefix string) *logger {
	return &logger{
		verbose: l.verbose,
		prefix:  prefix,
	}
}

// formatMessage formats a log message with key-value pairs
func (l *logger) formatMessage(level, msg string, keysAndValues ...interface{}) string {
	var sb strings.Builder
	
	// Timestamp
	sb.WriteString(time.Now().Format("2006-01-02T15:04:05.000Z07:00"))
	sb.WriteString(" ")
	
	// Level
	sb.WriteString(fmt.Sprintf("%-5s", level))
	sb.WriteString(" ")
	
	// Prefix if set
	if l.prefix != "" {
		sb.WriteString("[")
		sb.WriteString(l.prefix)
		sb.WriteString("] ")
	}
	
	// Message
	sb.WriteString(msg)
	
	// Key-value pairs
	if len(keysAndValues) > 0 {
		sb.WriteString(" ")
		for i := 0; i < len(keysAndValues); i += 2 {
			if i > 0 {
				sb.WriteString(" ")
			}
			
			key := fmt.Sprintf("%v", keysAndValues[i])
			var value interface{}
			if i+1 < len(keysAndValues) {
				value = keysAndValues[i+1]
			}
			
			sb.WriteString(key)
			sb.WriteString("=")
			sb.WriteString(fmt.Sprintf("%v", value))
		}
	}
	
	return sb.String()
}

// Debug logs a debug message (only in verbose mode)
func (l *logger) Debug(msg string, keysAndValues ...interface{}) {
	if l.verbose {
		log.Println(l.formatMessage("DEBUG", msg, keysAndValues...))
	}
}

// Info logs an info message
func (l *logger) Info(msg string, keysAndValues ...interface{}) {
	log.Println(l.formatMessage("INFO", msg, keysAndValues...))
}

// Error logs an error message
func (l *logger) Error(msg string, keysAndValues ...interface{}) {
	log.Println(l.formatMessage("ERROR", msg, keysAndValues...))
}

// Fatal logs a fatal message and exits
func (l *logger) Fatal(msg string, keysAndValues ...interface{}) {
	log.Println(l.formatMessage("FATAL", msg, keysAndValues...))
	os.Exit(1)
}

// Adapters for component loggers

// meshLogger adapts our logger to mesh.Logger interface
type meshLogger struct {
	*logger
}

func (m meshLogger) Warn(msg string, keysAndValues ...interface{}) {
	m.Info(msg, keysAndValues...)
}

// transportLogger adapts our logger to transport.Logger interface
type transportLogger struct {
	*logger
}

func (t transportLogger) Debugf(format string, args ...interface{}) {
	t.Debug(fmt.Sprintf(format, args...))
}

func (t transportLogger) Infof(format string, args ...interface{}) {
	t.Info(fmt.Sprintf(format, args...))
}

func (t transportLogger) Errorf(format string, args ...interface{}) {
	t.Error(fmt.Sprintf(format, args...))
}