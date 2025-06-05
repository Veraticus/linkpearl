# Production Hardening Implementation

This document describes the production hardening features implemented for the Linkpearl clipboard synchronization system.

## Overview

The production hardening implementation addresses critical reliability and security concerns for clipboard operations:

1. **Command Timeouts** - All external commands have configurable timeouts (default 5s)
2. **Resource Cleanup** - Proper cleanup of stdin pipes and processes in error paths
3. **Size Limits** - 10MB hard limit on clipboard content with UTF-8 validation
4. **Configuration** - Added CommandTimeout and MaxClipboardSize to sync.Config

## Implementation Details

### 1. Command Execution Hardening (`command.go`)

All clipboard operations now use centralized command execution functions:

```go
// RunCommand executes a command with timeout and resource management
func RunCommand(name string, args []string, config *CommandConfig) ([]byte, error)

// RunCommandWithInput executes a command with stdin input
func RunCommandWithInput(name string, args []string, input []byte, config *CommandConfig) error
```

Key features:
- Uses `exec.CommandContext` for proper timeout support
- Handles known exit codes (e.g., exit code 1 with empty output = empty clipboard)
- Enforces output size limits to prevent memory exhaustion
- Proper resource cleanup with defer statements

### 2. Content Validation (`limits.go`)

```go
// ValidateContent checks if clipboard content is within acceptable limits
func ValidateContent(content []byte) error
```

Validates:
- Content size (max 10MB)
- UTF-8 encoding for text content
- Returns wrapped errors for proper error handling

### 3. Platform Updates

Both `darwin.go` and `linux.go` have been updated to:
- Use the new command execution functions
- Apply timeouts to all clipboard operations
- Validate content before read/write operations
- Include timeout-specific error messages

### 4. Sync Engine Updates

The sync engine now:
- Checks content size before broadcasting to peers
- Validates incoming content from remote nodes
- Reports size limit violations in statistics
- Prevents propagation of oversized content

## Configuration

New configuration options in `sync.Config`:

```go
type Config struct {
    // ... existing fields ...
    
    // Timeout for clipboard operations (default: 5s)
    CommandTimeout time.Duration
    
    // Maximum clipboard content size in bytes (default: 10MB)
    MaxClipboardSize int
}
```

## Testing

Comprehensive test coverage includes:

1. **Command Tests** (`command_test.go`)
   - Timeout behavior
   - Error handling
   - Resource cleanup
   - Output size limits

2. **Limits Tests** (`limits_test.go`)
   - Size validation
   - UTF-8 validation
   - Error message format

3. **Integration Tests** (`integration_test.go`)
   - End-to-end timeout verification
   - Size limit enforcement
   - Concurrent operations
   - Invalid UTF-8 handling

## Error Handling

All errors are properly wrapped and include context:
- Command name and timeout duration in timeout errors
- Actual vs limit sizes in size errors
- Platform-specific error details

## Performance Impact

The hardening features have minimal performance impact:
- Timeout checks use context cancellation (efficient)
- Size validation is O(n) for UTF-8 check
- Command execution overhead is negligible
- No additional memory allocations in hot paths

## Migration Notes

The changes are backward compatible:
- Existing code continues to work
- Default values are applied automatically
- No API changes for clipboard interface

To customize behavior:
1. Set `CommandTimeout` in sync.Config for different timeout
2. Set `MaxClipboardSize` for different size limit
3. Use custom `CommandConfig` for fine-grained control

## Security Considerations

1. **Resource Exhaustion Prevention**
   - Size limits prevent memory exhaustion attacks
   - Timeouts prevent hanging on malicious/broken clipboard tools

2. **Data Validation**
   - UTF-8 validation prevents injection attacks
   - Size limits prevent DoS through large clipboard data

3. **Process Management**
   - Proper cleanup prevents process leaks
   - Kill processes on timeout to free resources