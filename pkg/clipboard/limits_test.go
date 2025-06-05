package clipboard

import (
	"bytes"
	"testing"
)

func TestValidateContent(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid small content",
			content: []byte("Hello, world!"),
			wantErr: false,
		},
		{
			name:    "valid empty content",
			content: []byte{},
			wantErr: false,
		},
		{
			name:    "valid large content under limit",
			content: bytes.Repeat([]byte("a"), MaxReasonableSize),
			wantErr: false,
		},
		{
			name:    "content at max size",
			content: bytes.Repeat([]byte("a"), MaxClipboardSize),
			wantErr: false,
		},
		{
			name:    "content exceeds max size",
			content: bytes.Repeat([]byte("a"), MaxClipboardSize+1),
			wantErr: true,
			errMsg:  "content exceeds maximum size",
		},
		{
			name:    "invalid UTF-8",
			content: []byte{0xff, 0xfe, 0xfd},
			wantErr: true,
			errMsg:  "invalid UTF-8",
		},
		{
			name:    "valid UTF-8 with special characters",
			content: []byte("Hello 世界 🌍"),
			wantErr: false,
		},
		{
			name:    "valid UTF-8 with newlines",
			content: []byte("Line 1\nLine 2\r\nLine 3"),
			wantErr: false,
		},
		{
			name:    "null bytes in content",
			content: []byte("Hello\x00World"),
			wantErr: false, // null bytes are valid UTF-8
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContent(tt.content)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("error %q doesn't contain expected string %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestMaxClipboardSizeConstant(t *testing.T) {
	// Verify the constant is set to expected value
	expectedSize := 10 * 1024 * 1024 // 10MB
	if MaxClipboardSize != expectedSize {
		t.Errorf("MaxClipboardSize = %d, want %d", MaxClipboardSize, expectedSize)
	}
}

func TestMaxReasonableSizeConstant(t *testing.T) {
	// Verify the constant is set to expected value
	expectedSize := 1024 * 1024 // 1MB
	if MaxReasonableSize != expectedSize {
		t.Errorf("MaxReasonableSize = %d, want %d", MaxReasonableSize, expectedSize)
	}
}

func BenchmarkValidateContent(b *testing.B) {
	// Benchmark with different content sizes
	sizes := []struct {
		name string
		size int
	}{
		{"Small", 100},
		{"Medium", 10 * 1024},
		{"Large", 1024 * 1024},
		{"VeryLarge", 5 * 1024 * 1024},
	}

	for _, size := range sizes {
		content := bytes.Repeat([]byte("a"), size.size)
		b.Run(size.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = ValidateContent(content)
			}
		})
	}
}
