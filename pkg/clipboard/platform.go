//go:build !darwin && !linux && !windows
// +build !darwin,!linux,!windows

package clipboard

// newPlatformClipboard returns an error for unsupported platforms
func newPlatformClipboard() (Clipboard, error) {
	return nil, ErrNotSupported
}
