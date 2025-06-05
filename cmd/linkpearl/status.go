package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/Veraticus/linkpearl/pkg/api"
	"github.com/Veraticus/linkpearl/pkg/client"
)

var (
	statusJSON bool

	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
		Long: `Display the current status of the linkpearl daemon.

Shows information about:
- Node ID and mode
- Connected peers
- Synchronization statistics
- Uptime and version

Examples:
  # Show status in human-readable format
  linkpearl status

  # Show status as JSON
  linkpearl status --json`,
		RunE: runStatus,
		Args: cobra.NoArgs,
	}
)

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output status as JSON")
}

func runStatus(_ *cobra.Command, _ []string) error {
	// Create client
	c := client.New(&client.Config{
		SocketPath: socketPath,
	})

	// Get status
	status, err := c.Status()
	if err != nil {
		return err
	}

	// Output based on format
	if statusJSON {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(status); err != nil {
			return fmt.Errorf("failed to encode status: %w", err)
		}
	} else {
		// Human-readable output
		printHumanStatus(status)
	}

	return nil
}

// printHumanStatus prints status in a human-readable format.
func printHumanStatus(status *api.StatusResponse) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	// Basic information
	_, _ = fmt.Fprintf(w, "Node ID:\t%s\n", status.NodeID)
	_, _ = fmt.Fprintf(w, "Mode:\t%s\n", status.Mode)
	_, _ = fmt.Fprintf(w, "Version:\t%s\n", status.Version)

	// Calculate uptime
	uptime := time.Since(status.Uptime)
	_, _ = fmt.Fprintf(w, "Uptime:\t%s\n", formatDuration(uptime))

	// Network information
	if status.ListenAddr != "" {
		_, _ = fmt.Fprintf(w, "Listen Address:\t%s\n", status.ListenAddr)
	}

	_, _ = fmt.Fprintf(w, "\nNetwork:\n")
	_, _ = fmt.Fprintf(w, "  Connected Peers:\t%d\n", len(status.ConnectedPeers))
	if len(status.ConnectedPeers) > 0 {
		for _, peer := range status.ConnectedPeers {
			_, _ = fmt.Fprintf(w, "    - %s\n", peer)
		}
	}

	if len(status.JoinAddresses) > 0 {
		_, _ = fmt.Fprintf(w, "  Join Addresses:\n")
		for _, addr := range status.JoinAddresses {
			_, _ = fmt.Fprintf(w, "    - %s\n", addr)
		}
	}

	// Sync statistics
	_, _ = fmt.Fprintf(w, "\nSynchronization:\n")
	_, _ = fmt.Fprintf(w, "  Messages Sent:\t%d\n", status.Stats.MessagesSent)
	_, _ = fmt.Fprintf(w, "  Messages Received:\t%d\n", status.Stats.MessagesReceived)
	_, _ = fmt.Fprintf(w, "  Local Changes:\t%d\n", status.Stats.LocalChanges)
	_, _ = fmt.Fprintf(w, "  Remote Changes:\t%d\n", status.Stats.RemoteChanges)

	if status.Stats.LastSyncTime != "" {
		if t, err := time.Parse(time.RFC3339, status.Stats.LastSyncTime); err == nil {
			ago := time.Since(t)
			_, _ = fmt.Fprintf(w, "  Last Sync:\t%s ago\n", formatDuration(ago))
		}
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}
