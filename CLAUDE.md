# Development Guide for Claude

This document provides important context and guidelines for Claude when working on the Linkpearl codebase.

## Project Overview

Linkpearl is a secure, peer-to-peer clipboard synchronization tool that creates a mesh network of computers. Key architectural decisions:

- **State-based synchronization**: Clipboard Watch() returns notifications (empty structs), not content
- **Pull-based design**: Consumers read clipboard content when ready, preventing blocking
- **Production hardening**: All external commands have timeouts, size limits, and proper error handling

## Pre-Commit Testing

**IMPORTANT**: Before committing any changes, run the comprehensive test suite:

```bash
./scripts/test-all.sh
```

This script performs:
1. Go formatting checks (gofmt -s)
2. go vet static analysis
3. golangci-lint (if installed)
4. Unit tests
5. Race detection tests
6. Integration tests
7. Test coverage analysis
8. **Cross-platform build verification** (Darwin/Linux, multiple architectures)
   - Binary builds for darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, linux/386
   - **Test compilation** for each platform to catch platform-specific syntax errors
9. Code quality checks (TODOs, fmt.Print usage)

The test script now catches platform-specific compilation errors that only appear when building for specific GOOS/GOARCH combinations.

## Common Issues to Avoid

### 1. Unused Imports
When refactoring code, especially platform-specific files (`darwin.go`, `linux.go`), ensure all imports are still used:
```bash
# Check each platform individually
GOOS=darwin go build ./...
GOOS=linux go build ./...
```

### 2. Platform-Specific Code
- Always test builds for both Darwin and Linux
- Use build tags correctly: `//go:build darwin` or `//go:build linux`
- Ensure platform-specific files have matching interfaces

### 3. Command Execution
All external commands MUST use the centralized command execution functions:
- Use `RunCommand()` for commands that return output
- Use `RunCommandWithInput()` for commands that need stdin
- Never use `exec.Command()` directly
- Always enforce timeouts (default 5s)

### 4. Size Limits
- Clipboard content is limited to 10MB (`MaxClipboardSize`)
- Always validate content with `ValidateContent()` before operations
- Check size limits in both local and remote operations

### 5. Error Handling
- Wrap errors with context using `fmt.Errorf()`
- Use the defined error variables (e.g., `ErrContentTooLarge`)
- Ensure errors can be checked with `errors.Is()`

## Testing Guidelines

### Unit Tests
- Mock external dependencies (clipboard, network)
- Test error conditions thoroughly
- Use table-driven tests for multiple scenarios
- Aim for >80% coverage on new code

### Integration Tests
- Use build tag: `//go:build integration`
- Skip gracefully when system resources aren't available
- Test real clipboard operations when possible
- Test multi-node scenarios

### Race Detection
Critical packages must pass race detection:
```bash
go test -race ./pkg/clipboard/...
go test -race ./pkg/sync/...
go test -race ./pkg/mesh/...
```

## Code Style

### Imports
Group imports in this order:
1. Standard library
2. External dependencies
3. Internal packages

### Comments
- Package comments explain the package purpose
- Exported functions need comprehensive godoc
- Complex logic needs inline explanation
- No TODO/FIXME in production code

### Logging
- Use the Logger interface, not fmt.Print*
- Include relevant context in log messages
- Use appropriate log levels (Debug, Info, Error)

## Quick Commands

```bash
# Format all code
gofmt -s -w .

# Run all tests
go test ./...

# Run linter
golangci-lint run ./...

# Build for current platform
go build ./cmd/linkpearl

# Cross-platform builds
GOOS=darwin GOARCH=amd64 go build ./cmd/linkpearl
GOOS=linux GOARCH=amd64 go build ./cmd/linkpearl

# Run specific package tests
go test -v ./pkg/clipboard/...

# Run with race detection
go test -race ./...

# Run integration tests
go test -tags=integration ./...

# Check test coverage
go test -cover ./...
```

## Architecture Reminders

1. **Clipboard Interface**: Read(), Write(), Watch(), GetState()
2. **Watch Behavior**: Returns `<-chan struct{}`, not content
3. **State Tracking**: SequenceNumber, LastModified, ContentHash
4. **Sync Engine**: Handles deduplication, conflict resolution, size limits
5. **Mesh Topology**: Static connections, no automatic peer discovery

## Production Hardening Checklist

- [ ] All commands use timeout context
- [ ] Resource cleanup in defer blocks
- [ ] Size validation on all inputs
- [ ] UTF-8 validation for text content
- [ ] Proper error wrapping and context
- [ ] No goroutine leaks
- [ ] No resource leaks (files, processes)
- [ ] Graceful degradation on failures
- [ ] Rate limiting where appropriate
- [ ] Metrics collection for observability

## Common Makefile Targets

```bash
make test       # Run all tests
make lint       # Run linters
make build      # Build binary
make clean      # Clean build artifacts
make coverage   # Generate coverage report
make test-builds # Test cross-platform builds and test compilation
make test-all   # Run comprehensive test suite (fmt, vet, lint, test, race, integration, builds)
```

The `test-builds` target now includes:
- Binary compilation for all supported platforms
- Test compilation for all platforms (catches platform-specific syntax errors)

Remember: Always run `./scripts/test-all.sh` or `make test-all` before committing!