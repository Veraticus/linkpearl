// Package main implements the linkpearl CLI application for secure P2P clipboard synchronization.
//
// # Overview
//
// Linkpearl is a distributed clipboard synchronization tool that enables secure sharing of clipboard
// content across multiple devices. It uses a peer-to-peer mesh network architecture where nodes can
// operate in two modes:
//
//   - Full Node: Accepts incoming connections and can relay messages between peers
//   - Client Node: Only makes outbound connections, suitable for NAT/firewall environments
//
// # CLI Structure
//
// The application follows a standard command-line interface pattern with:
//
//   - Flag-based configuration using Go's flag package
//   - Environment variable support for sensitive settings (e.g., LINKPEARL_SECRET)
//   - Structured logging with configurable verbosity levels
//   - Graceful shutdown handling with signal interception
//
// # Startup Sequence
//
// The application initialization follows this sequence:
//
//  1. Parse command-line flags and environment variables
//  2. Validate configuration (secret required, valid addresses)
//  3. Initialize platform-specific clipboard access
//  4. Create network transport layer with authentication
//  5. Build mesh topology for peer discovery and management
//  6. Start sync engine for clipboard synchronization
//  7. Begin main event loop
//
// # Graceful Shutdown
//
// The application handles shutdown gracefully through:
//
//   - Signal handling for SIGINT (Ctrl+C) and SIGTERM
//   - Context cancellation propagated to all components
//   - 10-second shutdown timeout to ensure cleanup
//   - Final statistics logging before exit
//
// All components implement proper cleanup in reverse initialization order to ensure
// resources are released correctly and data integrity is maintained.
//
// # Example Usage
//
//	# Start a full node that accepts connections
//	linkpearl --secret mysecret --listen :8080
//
//	# Start a client node connecting to a full node
//	linkpearl --secret mysecret --join server.example.com:8080
//
//	# Connect to multiple nodes for redundancy
//	linkpearl --secret mysecret --join host1:8080 --join host2:8080
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Veraticus/linkpearl/pkg/clipboard"
	"github.com/Veraticus/linkpearl/pkg/config"
	"github.com/Veraticus/linkpearl/pkg/mesh"
	"github.com/Veraticus/linkpearl/pkg/sync"
	"github.com/Veraticus/linkpearl/pkg/transport"
)

var (
	// Version information (set by build flags)
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// main is the application entry point. It handles command-line parsing, configuration validation,
// and orchestrates the startup of all application components.
func main() {
	// Create config with defaults
	cfg := config.NewConfig()
	
	// Define flags
	var joinAddrs stringSliceFlag
	
	flag.StringVar(&cfg.Secret, "secret", "", "Shared secret for linkshell (required)")
	flag.StringVar(&cfg.NodeID, "node-id", cfg.NodeID, "Node identifier (auto-generated if not set)")
	flag.StringVar(&cfg.Listen, "listen", "", "Listen address (e.g., :8080 or 0.0.0.0:8080)")
	flag.Var(&joinAddrs, "join", "Address to join (can be repeated or comma-separated)")
	flag.DurationVar(&cfg.PollInterval, "poll-interval", cfg.PollInterval, "Clipboard polling interval")
	flag.DurationVar(&cfg.ReconnectBackoff, "reconnect-backoff", cfg.ReconnectBackoff, "Initial reconnection backoff")
	flag.BoolVar(&cfg.Verbose, "v", false, "Enable verbose logging")
	
	// Version flag
	showVersion := flag.Bool("version", false, "Show version information")
	
	// Custom usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "linkpearl - Secure P2P clipboard synchronization\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Start a full node (accepts connections)\n")
		fmt.Fprintf(os.Stderr, "  %s --secret mysecret --listen :8080\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Start a client node (outbound only)\n")
		fmt.Fprintf(os.Stderr, "  %s --secret mysecret --join server.example.com:8080\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Join multiple nodes\n")
		fmt.Fprintf(os.Stderr, "  %s --secret mysecret --join host1:8080 --join host2:8080\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	
	flag.Parse()
	
	// Show version if requested
	if *showVersion {
		fmt.Printf("linkpearl version %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}
	
	// Load environment variables
	cfg.LoadFromEnv()
	
	// Set join addresses from flag
	if len(joinAddrs) > 0 {
		cfg.Join = joinAddrs.Get()
	}
	
	// Auto-determine mode based on listen flag
	if cfg.Listen != "" {
		cfg.Mode = config.FullNode
	} else {
		cfg.Mode = config.ClientNode
	}
	
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}
	
	// Create logger
	log := newLogger(cfg.Verbose)
	
	// Log startup info
	log.Info("starting linkpearl", 
		"version", version,
		"node_id", cfg.NodeID,
		"mode", cfg.Mode,
	)
	
	if cfg.Verbose {
		// Log configuration (without secret)
		log.Debug("configuration", "config", cfg.String())
	}
	
	// Run the application
	if err := run(cfg, log); err != nil {
		log.Fatal("failed to run", "error", err)
	}
}

// run executes the main application logic. It initializes all components in the correct order,
// starts background services, and manages the application lifecycle including graceful shutdown.
// The function blocks until a shutdown signal is received or a critical error occurs.
func run(cfg *config.Config, log *logger) error {
	// Create root context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	
	// Create clipboard
	log.Info("initializing clipboard")
	clip, err := createClipboard()
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
		if err := trans.Close(); err != nil {
			log.Error("failed to close transport", "error", err)
		}
	}()
	
	// Create topology
	log.Info("initializing mesh topology")
	topo, err := createTopology(cfg, trans, log)
	if err != nil {
		return fmt.Errorf("failed to create topology: %w", err)
	}
	defer func() {
		if err := topo.Stop(); err != nil {
			log.Error("failed to stop topology", "error", err)
		}
	}()
	
	// Start topology
	if err := topo.Start(ctx); err != nil {
		return fmt.Errorf("failed to start topology: %w", err)
	}
	
	// Add join addresses
	for _, addr := range cfg.Join {
		if err := topo.AddJoinAddr(addr); err != nil {
			log.Error("failed to add join address", "addr", addr, "error", err)
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
	
	// Start sync engine in background
	engineDone := make(chan error, 1)
	go func() {
		log.Info("sync engine started")
		engineDone <- engine.Run(ctx)
	}()
	
	// Log successful startup
	log.Info("linkpearl is running",
		"mode", cfg.Mode,
		"listen", cfg.Listen,
		"peers", len(cfg.Join),
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
	
	log.Info("linkpearl stopped")
	return nil
}

// createClipboard creates the appropriate clipboard implementation
func createClipboard() (clipboard.Clipboard, error) {
	// Use the platform clipboard factory
	clip, err := clipboard.NewPlatformClipboard()
	if err != nil {
		return nil, fmt.Errorf("failed to create platform clipboard: %w", err)
	}
	
	// Test clipboard access
	if _, err := clip.Read(); err != nil {
		return nil, fmt.Errorf("clipboard access test failed: %w", err)
	}
	
	return clip, nil
}

// createTransport creates the network transport
func createTransport(cfg *config.Config, log *logger) (transport.Transport, error) {
	// Create transport config
	transportCfg := &transport.TransportConfig{
		NodeID: cfg.NodeID,
		Mode:   string(cfg.Mode),
		Secret: cfg.Secret,
		Logger: transportLogger{log.withPrefix("transport")},
	}
	
	// Create TCP transport
	trans := transport.NewTCPTransport(transportCfg)
	
	// Listen if we're a full node
	if cfg.Mode == config.FullNode && cfg.Listen != "" {
		if err := trans.Listen(cfg.Listen); err != nil {
			return nil, fmt.Errorf("failed to listen on %s: %w", cfg.Listen, err)
		}
		log.Info("listening for connections", "addr", trans.Addr())
	}
	
	return trans, nil
}

// createTopology creates the mesh topology
func createTopology(cfg *config.Config, trans transport.Transport, log *logger) (mesh.Topology, error) {
	// Create topology config
	topoCfg := &mesh.TopologyConfig{
		Self: mesh.Node{
			ID:   cfg.NodeID,
			Mode: string(cfg.Mode),
			Addr: cfg.Listen,
		},
		Transport:             trans,
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

// createSyncEngine creates the sync engine
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

// init sets up logging format
func init() {
	// Remove timestamp from standard logger as we add our own
	log.SetFlags(0)
	
	// Color output support could be added here in the future
	// based on TERM environment variable
}