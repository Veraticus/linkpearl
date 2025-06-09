package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Veraticus/linkpearl/pkg/api"
	"github.com/Veraticus/linkpearl/pkg/clipboard"
	"github.com/Veraticus/linkpearl/pkg/config"
	"github.com/Veraticus/linkpearl/pkg/mesh"
	"github.com/Veraticus/linkpearl/pkg/sync"
	"github.com/Veraticus/linkpearl/pkg/transport"
)

var (
	// Run command flags.
	runCfg    config.Config
	joinAddrs []string

	runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run linkpearl daemon",
		Long: `Run the linkpearl daemon for clipboard synchronization.

This starts a background service that:
- Monitors the local clipboard for changes (if available)
- Synchronizes clipboard content with connected nodes  
- Provides a local API for the copy/paste commands

The daemon requires a shared secret for authentication. All nodes
in your mesh must use the same secret.

Note: If no clipboard tool is found (xsel, xclip, wl-clipboard on Linux,
or pbcopy/pbpaste on macOS), linkpearl will still run using an in-memory
clipboard. This allows it to work as a network pipe on headless servers.

The daemon runs continuously, monitoring local clipboard changes and 
synchronizing with connected peers. It also provides a local Unix socket 
API for client commands (copy, paste, status).

Examples:
  # Start a full node (accepts connections)
  linkpearl run --secret mysecret --listen :8080

  # Start a client node (outbound only)
  linkpearl run --secret mysecret --join server.example.com:8080

  # Join multiple nodes
  linkpearl run --secret mysecret --join host1:8080 --join host2:8080`,
		RunE: runDaemon,
	}
)

func init() {
	// Initialize with defaults
	runCfg = *config.NewConfig()

	// Daemon-specific flags
	runCmd.Flags().StringVar(&runCfg.Secret, "secret", "", "Shared secret for linkshell (required)")
	runCmd.Flags().StringVar(&runCfg.SecretFile, "secret-file", "", "Path to file containing the shared secret")
	runCmd.Flags().StringVar(&runCfg.NodeID, "node-id", runCfg.NodeID, "Node identifier (auto-generated if not set)")
	runCmd.Flags().StringVar(&runCfg.Listen, "listen", "", "Listen address (default: :9437 for full nodes)")
	runCmd.Flags().StringSliceVar(&joinAddrs, "join", []string{}, "Addresses to join (can be repeated)")
	runCmd.Flags().DurationVar(&runCfg.PollInterval, "poll-interval", runCfg.PollInterval, "Clipboard polling interval")
	runCmd.Flags().DurationVar(&runCfg.ReconnectBackoff, "reconnect-backoff", runCfg.ReconnectBackoff, "Initial reconnection backoff")
	runCmd.Flags().BoolVarP(&runCfg.Verbose, "verbose", "v", false, "Enable verbose logging")
}

func runDaemon(_ *cobra.Command, _ []string) error {
	// Load environment variables
	runCfg.LoadFromEnv()

	// Set join addresses from command line flags, but only if they were provided
	// This allows LINKPEARL_JOIN environment variable to work when no --join flags are given
	if len(joinAddrs) > 0 {
		runCfg.Join = joinAddrs
	}

	// Auto-determine mode based on listen flag
	if runCfg.Listen != "" {
		runCfg.Mode = config.FullNode
	} else {
		runCfg.Mode = config.ClientNode
	}

	// Validate configuration
	if err := runCfg.Validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Create logger
	log := newLogger(runCfg.Verbose)

	// Log startup info
	log.Info("starting linkpearl daemon",
		"version", version,
		"node_id", runCfg.NodeID,
		"mode", runCfg.Mode,
		"socket", socketPath,
	)

	if runCfg.Verbose {
		// Log configuration (without secret)
		log.Debug("configuration", "config", runCfg.String())
	}

	// Run the daemon
	return runDaemonWithConfig(&runCfg, log)
}

// runDaemonWithConfig executes the daemon with the given configuration.
// This is extracted to make testing easier.
func runDaemonWithConfig(cfg *config.Config, log *logger) error {
	// Create root context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Create clipboard
	log.Info("initializing clipboard")
	clip, err := createClipboard(log)
	if err != nil {
		return fmt.Errorf("failed to create clipboard: %w", err)
	}

	// Create transport
	log.Info("initializing transport")
	trans, err := createTransport(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}
	defer func() {
		if closeErr := trans.Close(); closeErr != nil {
			log.Error("failed to close transport", "error", closeErr)
		}
	}()

	// Create topology
	log.Info("initializing mesh topology")
	topo, err := createTopology(cfg, trans, log)
	if err != nil {
		return fmt.Errorf("failed to create topology: %w", err)
	}
	defer func() {
		if stopErr := topo.Stop(); stopErr != nil {
			log.Error("failed to stop topology", "error", stopErr)
		}
	}()

	// Start topology
	if startErr := topo.Start(ctx); startErr != nil {
		return fmt.Errorf("failed to start topology: %w", startErr)
	}

	// Add join addresses
	for _, addr := range cfg.Join {
		if addErr := topo.AddJoinAddr(addr); addErr != nil {
			log.Error("failed to add join address", "addr", addr, "error", addErr)
		} else {
			log.Info("added join address", "addr", addr)
		}
	}

	// Create sync engine
	log.Info("initializing sync engine")
	engine, err := createSyncEngine(cfg, clip, topo, log)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}

	// Create and start API server
	log.Info("initializing API server", "socket", socketPath)
	
	// Create slog logger for API server
	var slogLogger *slog.Logger
	if cfg.Verbose {
		slogLogger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	} else {
		slogLogger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}
	
	apiServer, err := api.NewServer(&api.ServerConfig{
		SocketPath: socketPath,
		Clipboard:  clip,
		Engine:     engine,
		NodeID:     cfg.NodeID,
		Mode:       string(cfg.Mode),
		Version:    version,
		Logger:     slogLogger.With("component", "api"),
	})
	if err != nil {
		return fmt.Errorf("failed to create API server: %w", err)
	}

	if err := apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}
	defer func() {
		if err := apiServer.Stop(); err != nil {
			log.Error("failed to stop API server", "error", err)
		}
	}()

	// Start sync engine in background
	engineDone := make(chan error, 1)
	go func() {
		log.Info("sync engine started")
		engineDone <- engine.Run(ctx)
	}()

	// Log successful startup
	log.Info("linkpearl daemon is running",
		"mode", cfg.Mode,
		"listen", cfg.Listen,
		"peers", len(cfg.Join),
		"socket", socketPath,
	)

	// Wait for shutdown signal or engine error
	select {
	case sig := <-sigCh:
		log.Info("received signal, shutting down", "signal", sig)
		cancel()

	case err := <-engineDone:
		if err != nil {
			log.Error("sync engine error", "error", err)
			return err
		}
		log.Info("sync engine stopped")
	}

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Wait for engine to stop
	select {
	case <-engineDone:
		log.Debug("sync engine stopped gracefully")
	case <-shutdownCtx.Done():
		log.Error("sync engine shutdown timeout")
	}

	// Log final stats
	stats := engine.Stats()
	log.Info("final statistics",
		"messages_sent", stats.MessagesSent,
		"messages_received", stats.MessagesReceived,
		"local_changes", stats.LocalChanges,
		"remote_changes", stats.RemoteChanges,
		"uptime", time.Since(stats.StartTime),
	)

	log.Info("linkpearl daemon stopped")
	return nil
}

// createClipboard creates the appropriate clipboard implementation.
func createClipboard(log *logger) (clipboard.Clipboard, error) {
	// Try to use the platform clipboard factory
	clip, err := clipboard.NewPlatformClipboard()
	if err != nil {
		// Log warning but don't fail - use noop clipboard instead
		log.Error("WARNING: No clipboard tool found, using in-memory clipboard",
			"error", err,
			"solution", "Install xsel, xclip, or wl-clipboard for system clipboard integration",
			"note", "linkpearl will still work for network synchronization")
		return clipboard.NewNoopClipboard(), nil
	}

	// Test clipboard access
	if _, err := clip.Read(); err != nil {
		// If clipboard exists but can't be accessed, fall back to noop
		log.Error("WARNING: Clipboard access failed, using in-memory clipboard",
			"error", err,
			"solution", "Check clipboard tool permissions and X11/Wayland access",
			"note", "linkpearl will still work for network synchronization")
		return clipboard.NewNoopClipboard(), nil
	}

	return clip, nil
}

// createTransport creates the network transport.
func createTransport(cfg *config.Config, log *logger) (transport.Transport, error) {
	// Create transport config
	transportCfg := &transport.Config{
		NodeID: cfg.NodeID,
		Mode:   string(cfg.Mode),
		Secret: cfg.Secret,
		Logger: transportLogger{log.withPrefix("transport")},
	}

	// Create TCP transport
	trans := transport.NewTCPTransport(transportCfg)

	// Note: The topology will handle listening when it starts
	return trans, nil
}

// createTopology creates the mesh topology.
func createTopology(cfg *config.Config, trans transport.Transport, log *logger) (mesh.Topology, error) {
	// Create topology config
	topoCfg := &mesh.TopologyConfig{
		Self: mesh.Node{
			ID:   cfg.NodeID,
			Mode: string(cfg.Mode),
			Addr: cfg.Listen,
		},
		Transport:            trans,
		JoinAddrs:            cfg.Join,
		ReconnectInterval:    cfg.ReconnectBackoff,
		MaxReconnectInterval: cfg.ReconnectBackoff * 300, // 5 minutes if backoff is 1s
		EventBufferSize:      1000,
		MessageBufferSize:    1000,
		Logger:               meshLogger{log.withPrefix("mesh")},
	}

	// Validate join addresses
	for _, addr := range topoCfg.JoinAddrs {
		if err := validateAddress(addr); err != nil {
			return nil, fmt.Errorf("invalid join address %q: %w", addr, err)
		}
	}

	// Create topology
	topo, err := mesh.NewTopology(topoCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create topology: %w", err)
	}

	return topo, nil
}

// createSyncEngine creates the sync engine.
func createSyncEngine(cfg *config.Config, clip clipboard.Clipboard, topo mesh.Topology, log *logger) (sync.Engine, error) {
	// Create sync config
	syncCfg := &sync.Config{
		NodeID:            cfg.NodeID,
		Clipboard:         clip,
		Topology:          topo,
		DedupeSize:        1000,
		SyncLoopWindow:    500 * time.Millisecond,
		MinChangeInterval: 100 * time.Millisecond,
		Logger:            log.withPrefix("sync"),
	}

	// Create sync engine
	engine, err := sync.NewEngine(syncCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync engine: %w", err)
	}

	return engine, nil
}
