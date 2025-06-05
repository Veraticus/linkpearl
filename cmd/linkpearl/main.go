// Package main implements the linkpearl CLI application for secure P2P clipboard synchronization.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version information (set by build flags).
	version = "dev"
	commit  = "none"
	date    = "unknown"

	// Global flags.
	socketPath string

	// rootCmd represents the base command when called without any subcommands.
	rootCmd = &cobra.Command{
		Use:   "linkpearl",
		Short: "Secure P2P clipboard synchronization",
		Long: `Linkpearl is a secure, peer-to-peer clipboard synchronization tool that 
creates a mesh network of computers, allowing seamless clipboard sharing 
between machines.

Named after Final Fantasy XIV's communication crystals, Linkpearl connects 
your devices in a "linkshell" for instant clipboard synchronization.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
)

func init() {
	// Remove timestamp from standard logger as we add our own
	log.SetFlags(0)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&socketPath, "socket", defaultSocketPath(),
		"Unix socket path for client-server communication")

	// Add subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(copyCmd)
	rootCmd.AddCommand(pasteCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// defaultSocketPath returns the default socket path based on XDG standards.
func defaultSocketPath() string {
	// Check XDG_RUNTIME_DIR first
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return xdg + "/linkpearl/linkpearl.sock"
	}
	// Fall back to home directory
	home := os.Getenv("HOME")
	if home == "" {
		home = "~"
	}
	return home + "/.linkpearl/linkpearl.sock"
}
