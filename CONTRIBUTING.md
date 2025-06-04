# Contributing to Linkpearl

Thank you for your interest in contributing to Linkpearl! This document provides guidelines and instructions for contributing to the project.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct:

- Be respectful and inclusive
- Welcome newcomers and help them get started
- Focus on constructive criticism
- Respect differing viewpoints and experiences

## How to Contribute

### Reporting Issues

Found a bug or have a feature request? Please open an issue with:

- Clear, descriptive title
- Steps to reproduce (for bugs)
- Expected vs actual behavior
- System information (OS, Go version)
- Relevant logs or error messages

### Submitting Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Make your changes** following our coding standards
3. **Add tests** for new functionality
4. **Update documentation** if needed
5. **Run tests** to ensure everything passes
6. **Submit a PR** with clear description of changes

### Development Setup

```bash
# Clone your fork
git clone https://github.com/yourusername/linkpearl.git
cd linkpearl

# Add upstream remote
git remote add upstream https://github.com/original/linkpearl.git

# Install dependencies
go mod download

# Run tests
make test

# Build binary
make build
```

## Coding Standards

### Go Style Guide

- Follow standard Go conventions and idioms
- Run `gofmt` on all code
- Use `golangci-lint` for additional checks
- Keep functions small and focused
- Prefer composition over inheritance

### Code Organization

```
linkpearl/
├── cmd/           # Application entry points
├── pkg/           # Public packages
│   ├── clipboard/ # Platform clipboard access
│   ├── transport/ # Network communication
│   ├── mesh/      # P2P topology
│   ├── sync/      # Synchronization engine
│   └── config/    # Configuration
└── internal/      # Private packages (if needed)
```

### Testing

- Write unit tests for all new functionality
- Use table-driven tests where appropriate
- Mock external dependencies
- Aim for >80% code coverage
- Include integration tests with build tags

Example test structure:
```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "test", "TEST", false},
        {"empty input", "", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Feature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Feature() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Feature() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Documentation

- Add package-level documentation
- Document all exported types and functions
- Include examples for complex functionality
- Update README.md for user-facing changes

Example documentation:
```go
// Package clipboard provides platform-agnostic access to the system clipboard.
// It supports reading, writing, and watching for clipboard changes on macOS,
// Linux, and Windows (coming soon).
package clipboard

// Clipboard defines the interface for clipboard operations.
// Implementations must be safe for concurrent use.
type Clipboard interface {
    // Read returns the current clipboard contents as a string.
    // Returns an error if the clipboard is empty or inaccessible.
    Read() (string, error)
    
    // Write sets the clipboard contents to the given string.
    // Returns an error if the operation fails.
    Write(content string) error
    
    // Watch returns a channel that emits clipboard content on changes.
    // The channel is closed when the context is cancelled.
    Watch(ctx context.Context) <-chan string
}
```

## Implementation Guidelines

### Adding Platform Support

To add support for a new platform:

1. Create platform-specific file: `pkg/clipboard/platform.go`
2. Implement the `Clipboard` interface
3. Add build tags: `// +build platform`
4. Update `NewPlatformClipboard()` factory
5. Add platform-specific tests
6. Update documentation

### Adding Features

Before implementing new features:

1. Discuss in an issue first
2. Keep backward compatibility
3. Follow existing patterns
4. Add configuration options if needed
5. Document behavior changes

### Performance Considerations

- Minimize allocations in hot paths
- Use buffered channels appropriately
- Profile code for performance issues
- Benchmark critical operations

## Pull Request Process

1. **Update your fork**
   ```bash
   git fetch upstream
   git checkout main
   git merge upstream/main
   ```

2. **Create feature branch**
   ```bash
   git checkout -b feature/your-feature
   ```

3. **Make changes and commit**
   ```bash
   git add .
   git commit -m "feat: add new feature"
   ```

4. **Run all checks**
   ```bash
   make fmt        # Format code
   make lint       # Run linters
   make test       # Run tests
   make test-race  # Run with race detector
   ```

5. **Push and create PR**
   ```bash
   git push origin feature/your-feature
   ```

### Commit Message Format

Follow conventional commits:

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `style:` Code style changes
- `refactor:` Code refactoring
- `test:` Test additions/changes
- `chore:` Build process or auxiliary tool changes

Example:
```
feat: add Windows clipboard support

- Implement Windows clipboard using win32 API
- Add clipboard change notifications via WM_CLIPBOARDUPDATE
- Update documentation for Windows requirements

Closes #123
```

## Release Process

1. Update version in code
2. Update CHANGELOG.md
3. Create release tag
4. Build binaries for all platforms
5. Create GitHub release with binaries

## Getting Help

- Check existing issues and discussions
- Join our community chat (if available)
- Ask questions in GitHub Discussions
- Tag maintainers for urgent issues

## Recognition

Contributors are recognized in:
- CONTRIBUTORS.md file
- Release notes
- Project documentation

Thank you for contributing to Linkpearl! 🔮