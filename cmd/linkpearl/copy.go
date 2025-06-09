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

var (
	copySync bool
)

var copyCmd = &cobra.Command{
	Use:   "copy [text]",
	Short: "Copy text to clipboard",
	Long: `Copy text to the clipboard via the linkpearl daemon.

If text is provided as an argument, it will be copied directly.
If no argument is provided, text will be read from stdin.

By default, copy operations are asynchronous (fire-and-forget) for better performance.
Use --sync to wait for confirmation from the daemon.

This command requires the linkpearl daemon to be running.

Examples:
  # Copy text directly (async by default)
  linkpearl copy "Hello, World!"

  # Copy from stdin
  echo "Hello, World!" | linkpearl copy

  # Copy file contents
  cat file.txt | linkpearl copy

  # Copy command output
  ls -la | linkpearl copy

  # Copy with confirmation (synchronous mode)
  linkpearl copy --sync "Hello, World!"`,
	RunE: runCopy,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	copyCmd.Flags().BoolVar(&copySync, "sync", false, "Wait for confirmation from daemon (default: async)")
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

	// Check for environment variable override for sync mode
	if os.Getenv("LINKPEARL_SYNC") == "1" {
		copySync = true
	}

	// Create client
	c := client.New(&client.Config{
		SocketPath: socketPath,
	})

	// Copy to clipboard
	// Default is async (fire-and-forget) for better performance
	if copySync {
		if err := c.Copy(content); err != nil {
			return err
		}
	} else {
		if err := c.CopyAsync(content); err != nil {
			return err
		}
	}

	// Success - no output for Unix philosophy
	return nil
}
