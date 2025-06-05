package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Veraticus/linkpearl/pkg/client"
)

var pasteCmd = &cobra.Command{
	Use:   "paste",
	Short: "Paste clipboard contents",
	Long: `Output the current clipboard contents from the linkpearl daemon.

This command requires the linkpearl daemon to be running.

Examples:
  # Paste to stdout
  linkpearl paste

  # Paste to file
  linkpearl paste > output.txt

  # Paste and process
  linkpearl paste | grep "pattern"`,
	RunE: runPaste,
	Args: cobra.NoArgs,
}

func runPaste(_ *cobra.Command, _ []string) error {
	// Create client
	c := client.New(&client.Config{
		SocketPath: socketPath,
	})

	// Get clipboard content
	content, err := c.Paste()
	if err != nil {
		// Check if this is a NeoVim integration error that should be silent
		if err == nil {
			return nil
		}
		return err
	}

	// Output to stdout (no newline added - preserve exact content)
	if _, err := fmt.Fprint(os.Stdout, content); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}
