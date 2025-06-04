# Integration Tests

This directory contains end-to-end integration tests for the linkpearl clipboard synchronization system.

## Test Files

### `integration_test.go`
Main integration test suite that tests the complete clipboard synchronization workflow between multiple nodes using mock clipboards. This file includes:

- **Two-node synchronization**: Basic sync between two nodes
- **Three-node mesh**: Full mesh topology testing
- **Client-server topology**: Hub-and-spoke configuration
- **Failure recovery**: Node disconnection and reconnection
- **Concurrent changes**: Conflict resolution testing
- **Large content**: Synchronization of large clipboard data
- **Rapid changes**: Handling of frequent clipboard updates
- **Benchmarks**: Performance measurements

### `system_integration_test.go`
Integration tests that use real system clipboards (when available). These tests are more environment-dependent and are skipped in CI. Includes:

- **System clipboard watch**: Tests real clipboard monitoring
- **End-to-end sync**: Full sync with actual system clipboards
- **Performance testing**: Real-world latency measurements

## Running Integration Tests

### Basic Integration Tests
Run all integration tests with mock clipboards:
```bash
go test -tags=integration ./tests/...
```

### System Integration Tests
Run tests with real system clipboards (requires display):
```bash
go test -tags="integration,!ci" ./tests/...
```

### Verbose Output
See detailed test progress:
```bash
go test -tags=integration -v ./tests/...
```

### Specific Test
Run a single test function:
```bash
go test -tags=integration -run TestEndToEndClipboardSync/TwoNodeSync ./tests/...
```

### With Coverage
Generate coverage report:
```bash
go test -tags=integration -cover ./tests/...
```

### Benchmarks
Run performance benchmarks:
```bash
go test -tags=integration -bench=. ./tests/...
```

## Build Tags

The tests use the following build tags:

- `integration`: Required for all integration tests
- `!ci`: Excludes tests that require system clipboard access (for CI environments)

## Prerequisites

### For Mock Integration Tests
- Go 1.21 or later
- Network access (tests use local TCP connections)

### For System Integration Tests
- All of the above, plus:
- **macOS**: Terminal must have accessibility permissions
- **Linux**: X11 or Wayland display, clipboard tools (xsel/xclip/wl-clipboard)
- **Windows**: Not yet supported

### Linux Clipboard Tools
Install required tools for clipboard access:
```bash
# Debian/Ubuntu (X11)
sudo apt install xsel xclip

# Debian/Ubuntu (Wayland)
sudo apt install wl-clipboard

# For efficient clipboard monitoring
sudo apt install clipnotify
```

## Environment Variables

- `CI`: Set this to skip system clipboard tests in CI environments
- `DISPLAY`: Required on Linux for X11 clipboard access
- `WAYLAND_DISPLAY`: Required on Linux for Wayland clipboard access

## Test Scenarios

### 1. Basic Two-Node Sync
Tests fundamental clipboard synchronization between two nodes:
- Node A writes to clipboard
- Verify Node B receives the update
- Node B writes to clipboard
- Verify Node A receives the update

### 2. Three-Node Mesh
Tests broadcast in a fully connected mesh:
- All nodes connect to each other
- Any node's clipboard change propagates to all others
- Verify no message loops or duplicates

### 3. Client-Server Topology
Tests hub-and-spoke configuration:
- One server node accepts connections
- Multiple client nodes connect only to server
- Clipboard changes flow through the server

### 4. Failure Recovery
Tests resilience to network failures:
- Establish connection between nodes
- Simulate node failure/disconnection
- Make clipboard changes while disconnected
- Verify sync resumes after reconnection

### 5. Concurrent Changes
Tests last-write-wins conflict resolution:
- Multiple nodes update clipboard simultaneously
- Verify all nodes converge to the same value
- Check that the latest timestamp wins

### 6. Large Content
Tests synchronization of large clipboard data:
- Generate 100KB+ of clipboard content
- Verify complete and correct synchronization
- Measure sync time for large payloads

### 7. Rapid Changes
Tests handling of frequent updates:
- Make rapid successive clipboard changes
- Verify deduplication works correctly
- Ensure final state is synchronized

## Troubleshooting

### "System clipboard not available"
- Ensure you have a display available (not running in SSH without X forwarding)
- Install required clipboard tools for your platform
- Check terminal permissions on macOS

### "Timeout waiting for clipboard change"
- Increase timeout values if running on slow systems
- Check that clipboard monitoring is working with `pbpaste -h` (macOS) or `xsel` (Linux)
- Verify no other applications are interfering with clipboard

### "Connection refused"
- Check that nodes are starting on different ports
- Verify no firewall blocking local connections
- Ensure transport layer is properly initialized

## Performance Expectations

Based on the benchmarks, expect:
- **Local network latency**: < 100ms for small content
- **Sync time for 100KB**: < 1 second
- **CPU usage during sync**: < 5% per node
- **Memory usage**: ~10-20MB per node