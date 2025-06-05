package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Veraticus/linkpearl/pkg/client"
)

var copyCmd = &cobra.Command{
	Use:   "copy [text]",
	Short: "Copy text to clipboard",
	Long: `Copy text to the clipboard via the linkpearl daemon.

If text is provided as an argument, it will be copied directly.
If no argument is provided, text will be read from stdin.

This command requires the linkpearl daemon to be running.

Examples:
  # Copy text directly
  linkpearl copy "Hello, World!"

  # Copy from stdin
  echo "Hello, World!" | linkpearl copy

  # Copy file contents
  cat file.txt | linkpearl copy

  # Copy command output
  ls -la | linkpearl copy`,
	RunE: runCopy,
	Args: cobra.MaximumNArgs(1),
}

func runCopy(_ *cobra.Command, args []string) error {
	var content string

	// Determine content source
	if len(args) > 0 {
		// Content provided as argument
		content = args[0]
	} else {
		// Read from stdin
		reader := bufio.NewReader(os.Stdin)
		var builder strings.Builder

		// Read all input
		for {
			buf := make([]byte, 4096)
			n, err := reader.Read(buf)
			if n > 0 {
				builder.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
		}

		content = builder.String()

		// Handle empty input
		if content == "" {
			return fmt.Errorf("no content to copy")
		}
	}

	// Create client
	c := client.New(&client.Config{
		SocketPath: socketPath,
	})

	// Copy to clipboard
	if err := c.Copy(content); err != nil {
		// Check if this is a NeoVim integration error that should be silent
		if err == nil {
			return nil
		}
		return err
	}

	// Success - no output for Unix philosophy
	return nil
}
