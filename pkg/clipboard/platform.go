//go:build !darwin && !linux && !windows
// +build !darwin,!linux,!windows

// This file provides a fallback implementation for unsupported platforms.
//
// When building for platforms other than macOS, Linux, or Windows, this
// implementation is used. It simply returns ErrNotSupported to indicate
// that clipboard functionality is not available on the current platform.
//
// Supported platforms are implemented in their respective files:
//   - darwin.go: macOS implementation using pbcopy/pbpaste
//   - linux.go: Linux implementation supporting X11 and Wayland
//   - windows.go: Windows implementation (if implemented)
//
// To add support for a new platform:
//   1. Create a new file with appropriate build tags (e.g., "freebsd.go")
//   2. Implement the newPlatformClipboard() function
//   3. Return a platform-specific implementation of the Clipboard interface
//   4. Update the build tags in this file to exclude your new platform

package clipboard

// newPlatformClipboard returns an error for unsupported platforms.
// This function is only compiled when building for platforms other than
// macOS, Linux, or Windows.
func newPlatformClipboard() (Clipboard, error) {
	return nil, ErrNotSupported
}
