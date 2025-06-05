// Package testutil provides common test utilities for the linkpearl project.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// SocketPath creates a short socket path to avoid macOS path length limits.
// macOS has a 104 character limit for Unix domain socket paths, while Linux has 108.
// This function creates paths directly in /tmp to keep them short.
func SocketPath(t *testing.T) string {
	t.Helper()

	// Use /tmp directly with a unique name based on test name and process ID
	// This keeps paths short even with long test names
	name := fmt.Sprintf("lp-%d-%d.sock", os.Getpid(), time.Now().UnixNano()%100000)
	path := filepath.Join("/tmp", name)

	// Register cleanup
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	return path
}
