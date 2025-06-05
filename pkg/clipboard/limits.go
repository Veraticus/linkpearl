package clipboard

import (
	"fmt"
	"unicode/utf8"
)

const (
	// MaxReasonableSize for normal text content (1MB).
	MaxReasonableSize = 1024 * 1024
)

// ValidateContent checks if clipboard content is within acceptable limits.
func ValidateContent(content []byte) error {
	if len(content) > MaxClipboardSize {
		return fmt.Errorf("%w: %d bytes (max: %d)",
			ErrContentTooLarge, len(content), MaxClipboardSize)
	}

	// TODO: Warn for large but valid content
	// if len(content) > MaxReasonableSize {
	//     logger.Warn("Large clipboard content", "size", len(content))
	// }

	// Ensure valid UTF-8 for text operations
	if !utf8.Valid(content) {
		return fmt.Errorf("clipboard content contains invalid UTF-8")
	}

	return nil
}
