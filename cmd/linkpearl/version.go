package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Display version information about linkpearl.`,
	Run: func(_ *cobra.Command, _ []string) {
		_, _ = fmt.Fprintf(os.Stdout, "linkpearl version %s\n", version)
		_, _ = fmt.Fprintf(os.Stdout, "  commit: %s\n", commit)
		_, _ = fmt.Fprintf(os.Stdout, "  built:  %s\n", date)
	},
	Args: cobra.NoArgs,
}
