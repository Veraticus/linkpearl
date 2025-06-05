# Development Guide for Claude

This document provides important context and guidelines for Claude when working on the Linkpearl codebase.

## Project Overview

Linkpearl is a secure, peer-to-peer clipboard synchronization tool that creates a mesh network of computers. Key architectural decisions:

- **State-based synchronization**: Clipboard Watch() returns notifications (empty structs), not content
- **Pull-based design**: Consumers read clipboard content when ready, preventing blocking
- **Production hardening**: All external commands have timeouts, size limits, and proper error handling

## Pre-Commit Testing & Code Quality Enforcement

**MANDATORY**: Before committing ANY changes, you MUST run verification:

```bash
# Quick verification (MINIMUM requirement before any commit)
make verify

# Full test suite (REQUIRED before pull requests)
make test-all
# OR
./scripts/test-all.sh
```

### Development Workflow

1. **Before starting work**: Install development tools
   ```bash
   make install-tools    # Install golangci-lint and other required tools
   make setup-hooks      # Setup git pre-commit hooks (one-time)
   ```

2. **During development**: Use these helpers
   ```bash
   make quick           # Format code and run tests
   make fix             # Auto-fix formatting, imports, and common issues
   make lint            # Run golangci-lint to catch quality issues
   ```

3. **Before committing**: ALWAYS run verification
   ```bash
   make verify          # Fast verification checks (MINIMUM)
   ```

4. **Before pull requests**: Run comprehensive tests
   ```bash
   make test-all        # Full test suite including cross-platform builds
   ```

### What These Scripts Check

The `verify` script (quick checks):
- Code formatting (gofmt -s)
- Go vet static analysis
- golangci-lint (comprehensive linting)
- Quick unit tests

The `test-all` script (comprehensive):
1. Go formatting checks (gofmt -s)
2. go vet static analysis
3. golangci-lint with custom rules
4. Unit tests
5. Race detection tests
6. Integration tests
7. Test coverage analysis
8. **Cross-platform build verification** (Darwin/Linux, multiple architectures)
   - Binary builds for darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, linux/386
   - **Test compilation** for each platform to catch platform-specific syntax errors
9. Nix build verification (if nix is installed)
10. Code quality checks (TODOs, fmt.Print usage)

The test scripts catch platform-specific compilation errors that only appear when building for specific GOOS/GOARCH combinations.

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
- Never use `exec.Command()` directly (golangci-lint will catch this!)
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
# DEVELOPMENT WORKFLOW (use these!)
make quick          # Format and test quickly
make fix            # Auto-fix common issues
make verify         # Pre-commit verification
make test-all       # Comprehensive test suite

# Individual operations
make fmt            # Format all code
make lint           # Run golangci-lint
make test           # Run unit tests
make test-race      # Run with race detection
make test-integration # Run integration tests
make cover          # Generate HTML coverage report

# Build operations
make build          # Build for current platform
make build-all      # Build for all platforms
make install        # Install binary locally

# Cross-platform builds (manual)
GOOS=darwin GOARCH=amd64 go build ./cmd/linkpearl
GOOS=linux GOARCH=amd64 go build ./cmd/linkpearl
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

## Essential Makefile Targets

### For Daily Development
```bash
make quick       # Format and test - use this frequently!
make fix         # Auto-fix issues (formatting, imports, etc.)
make verify      # Pre-commit checks - ALWAYS run before committing!
make lint        # Run golangci-lint to catch issues early
make cover       # Generate HTML coverage report
```

### Before Pull Requests
```bash
make test-all    # Comprehensive test suite - REQUIRED before PRs!
```

### Other Useful Targets
```bash
make build       # Build binary
make build-all   # Build for all platforms
make clean       # Clean build artifacts
make install     # Install binary
make test        # Run unit tests
make test-race   # Run with race detection
make test-integration # Run integration tests
make test-builds # Test cross-platform builds
make test-nix    # Test nix build
make install-tools # Install dev tools (golangci-lint, etc.)
make setup-hooks # Setup git pre-commit hooks
```

## CRITICAL REMINDERS

1. **ALWAYS run `make verify` before ANY commit** - This is non-negotiable!
2. **ALWAYS run `make test-all` before creating pull requests**
3. **Use `make fix` to automatically fix common issues**
4. **golangci-lint is configured to catch:**
   - Direct use of `exec.Command()` (use `RunCommand()` instead)
   - Use of `interface{}` (use `any` instead)
   - Use of `panic()` (return errors instead)
   - Use of `fmt.Print*` (use Logger interface)
   - Many other code quality issues

The build system will catch platform-specific compilation errors that only appear when building for specific GOOS/GOARCH combinations.

Remember: The tools are here to help you ship high-quality code. Use them!