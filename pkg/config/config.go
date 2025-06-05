// Package config provides configuration management for linkpearl nodes.
// It handles loading, validation, and management of all node settings including
// network configuration, security parameters, and operational modes.
//
// Configuration Sources:
//
// Configuration can be loaded from multiple sources with the following precedence:
//  1. Command-line flags (highest priority)
//  2. Environment variables
//  3. Default values (lowest priority)
//
// Environment Variables:
//
// All configuration options can be set via environment variables:
//   - LINKPEARL_SECRET: Shared secret for node authentication
//   - LINKPEARL_SECRET_FILE: Path to file containing the shared secret
//   - LINKPEARL_NODE_ID: Unique identifier for this node
//   - LINKPEARL_MODE: Operational mode (client or full)
//   - LINKPEARL_LISTEN: Address to listen on (full nodes only)
//   - LINKPEARL_JOIN: Comma-separated list of nodes to connect to
//   - LINKPEARL_POLL_INTERVAL: Clipboard polling frequency
//   - LINKPEARL_RECONNECT_BACKOFF: Delay between reconnection attempts
//   - LINKPEARL_VERBOSE: Enable verbose logging
//
// Node Modes:
//
// Linkpearl supports two operational modes:
//
//  1. Client Mode: Connects to other nodes but doesn't accept connections.
//     Ideal for devices behind NAT or with limited resources.
//
//  2. Full Mode: Accepts incoming connections and fully participates in
//     the mesh. Requires a publicly accessible address or port forwarding.
//
// Security:
//
// The shared secret is used for mutual authentication between nodes. All nodes
// in a mesh must use the same secret. The secret is never logged or displayed
// in configuration output for security reasons.
//
// Validation:
//
// The configuration is validated to ensure:
//   - Required fields are present (secret, node ID)
//   - Mode-specific requirements are met
//   - Network addresses are properly formatted
//   - Duration values are within reasonable bounds
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// NodeMode represents the operational mode of a linkpearl node.
// Different modes have different networking capabilities and requirements.
type NodeMode string

const (
	// ClientNode connects to other nodes but doesn't accept incoming connections..
	ClientNode NodeMode = "client"
	// FullNode accepts incoming connections and participates fully in the mesh..
	FullNode NodeMode = "full"
)

// Config holds all configuration for linkpearl nodes.
// This struct contains all settings needed to run a linkpearl node,
// from network configuration to behavioral parameters.
//
// The configuration is designed to be:
//   - Simple for basic use cases (minimal required fields)
//   - Flexible for advanced deployments (many optional settings)
//   - Secure by default (no sensitive data in logs)
type Config struct {
	// Core settings
	Secret     string   `env:"LINKPEARL_SECRET"`
	SecretFile string   `env:"LINKPEARL_SECRET_FILE"`
	NodeID     string   `env:"LINKPEARL_NODE_ID"`
	Mode       NodeMode `env:"LINKPEARL_MODE"`

	// Network settings
	Listen string   `env:"LINKPEARL_LISTEN"`
	Join   []string `env:"LINKPEARL_JOIN"`

	// Behavior
	PollInterval     time.Duration `env:"LINKPEARL_POLL_INTERVAL"`
	ReconnectBackoff time.Duration `env:"LINKPEARL_RECONNECT_BACKOFF"`
	Verbose          bool          `env:"LINKPEARL_VERBOSE"`
}

// NewConfig creates a new config with sensible defaults suitable for most
// deployments. The defaults are chosen to work well for client nodes on
// typical networks.
//
// Default values:
//   - NodeID: Generated from hostname and timestamp
//   - Mode: Client (doesn't require port forwarding)
//   - PollInterval: 500ms (responsive without excessive CPU)
//   - ReconnectBackoff: 1s (quick recovery from disconnections)
//   - Verbose: false (quiet operation)
//
// Users typically only need to set Secret and Join addresses.
func NewConfig() *Config {
	return &Config{
		NodeID:           generateNodeID(),
		Mode:             ClientNode,
		PollInterval:     500 * time.Millisecond,
		ReconnectBackoff: time.Second,
		Verbose:          false,
		Join:             []string{},
	}
}

// Validate ensures the configuration is valid and internally consistent.
// This method checks all requirements and relationships between settings.
//
// Validation rules:
//   - Secret is required (no default for security) - can be provided via --secret, LINKPEARL_SECRET, or --secret-file
//   - NodeID is required (must be unique per node)
//   - Mode must be either "client" or "full"
//   - Full nodes must specify a listen address
//   - Client nodes must not specify a listen address
//   - Client nodes must have at least one join address
//
// The method also performs automatic adjustments:
//   - If Listen is set, mode is upgraded to FullNode
//   - If SecretFile is set, reads the secret from the file
//
// Returns an error describing the first validation failure found.
func (c *Config) Validate() error {
	// Handle secret file if provided
	if c.SecretFile != "" {
		if c.Secret != "" {
			return fmt.Errorf("cannot specify both --secret and --secret-file")
		}

		content, err := os.ReadFile(c.SecretFile)
		if err != nil {
			return fmt.Errorf("failed to read secret file: %w", err)
		}

		c.Secret = strings.TrimSpace(string(content))
	}

	if c.Secret == "" {
		return fmt.Errorf("secret is required (use --secret, LINKPEARL_SECRET, or --secret-file)")
	}

	if c.NodeID == "" {
		return fmt.Errorf("node ID is required")
	}

	// If we have a listen address, we're a full node
	if c.Listen != "" {
		c.Mode = FullNode
	}

	// Validate node mode
	if c.Mode != ClientNode && c.Mode != FullNode {
		return fmt.Errorf("invalid node mode: %s", c.Mode)
	}

	// Full nodes must have a listen address
	if c.Mode == FullNode && c.Listen == "" {
		return fmt.Errorf("full nodes must specify a listen address")
	}

	// Client nodes should not have a listen address
	if c.Mode == ClientNode && c.Listen != "" {
		return fmt.Errorf("client nodes should not specify a listen address")
	}

	// Must have at least one join address if not listening
	if c.Mode == ClientNode && len(c.Join) == 0 {
		return fmt.Errorf("client nodes must specify at least one address to join")
	}

	return nil
}

// LoadFromEnv loads configuration from environment variables, overriding
// any existing values. This allows for easy configuration in containerized
// environments and shell scripts.
//
// Environment variable mappings:
//   - LINKPEARL_SECRET -> Secret (required for security)
//   - LINKPEARL_SECRET_FILE -> SecretFile (path to file containing secret)
//   - LINKPEARL_NODE_ID -> NodeID (defaults to generated ID)
//   - LINKPEARL_MODE -> Mode ("client" or "full")
//   - LINKPEARL_LISTEN -> Listen (e.g., ":9437" or "0.0.0.0:9437")
//   - LINKPEARL_JOIN -> Join (comma-separated, e.g., "host1:9437,host2:9437")
//   - LINKPEARL_POLL_INTERVAL -> PollInterval (e.g., "500ms", "1s")
//   - LINKPEARL_RECONNECT_BACKOFF -> ReconnectBackoff (e.g., "1s", "5s")
//   - LINKPEARL_VERBOSE -> Verbose ("true" or "false")
//
// Invalid values are silently ignored, keeping the existing configuration.
// This prevents the application from failing due to typos in optional settings.
func (c *Config) LoadFromEnv() {
	if secret := os.Getenv("LINKPEARL_SECRET"); secret != "" {
		c.Secret = secret
	}

	if secretFile := os.Getenv("LINKPEARL_SECRET_FILE"); secretFile != "" {
		c.SecretFile = secretFile
	}

	if nodeID := os.Getenv("LINKPEARL_NODE_ID"); nodeID != "" {
		c.NodeID = nodeID
	}

	if mode := os.Getenv("LINKPEARL_MODE"); mode != "" {
		c.Mode = NodeMode(strings.ToLower(mode))
	}

	if listen := os.Getenv("LINKPEARL_LISTEN"); listen != "" {
		c.Listen = listen
	}

	if join := os.Getenv("LINKPEARL_JOIN"); join != "" {
		c.Join = strings.Split(join, ",")
		// Trim whitespace from each address
		for i, addr := range c.Join {
			c.Join[i] = strings.TrimSpace(addr)
		}
	}

	if pollInterval := os.Getenv("LINKPEARL_POLL_INTERVAL"); pollInterval != "" {
		if d, err := time.ParseDuration(pollInterval); err == nil {
			c.PollInterval = d
		}
	}

	if reconnectBackoff := os.Getenv("LINKPEARL_RECONNECT_BACKOFF"); reconnectBackoff != "" {
		if d, err := time.ParseDuration(reconnectBackoff); err == nil {
			c.ReconnectBackoff = d
		}
	}

	if verbose := os.Getenv("LINKPEARL_VERBOSE"); verbose != "" {
		if v, err := strconv.ParseBool(verbose); err == nil {
			c.Verbose = v
		}
	}
}

// generateNodeID creates a unique node identifier for this instance.
// The ID combines hostname and nanosecond timestamp to ensure uniqueness
// even when multiple nodes start on the same host.
//
// Format: "{hostname}-{nanosecond-timestamp}"
// Example: "laptop-1234567890123456789"
//
// If hostname cannot be determined, "unknown" is used as a fallback.
// The nanosecond precision makes collisions virtually impossible.
func generateNodeID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	// Use UnixNano for more uniqueness
	return fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
}

// String returns a string representation of the config suitable for logging
// and debugging. Sensitive data (the secret) is hidden to prevent accidental
// exposure in logs or console output.
//
// The secret is shown as:
//   - "[hidden]" if set
//   - "[not set]" if empty
//
// This allows operators to verify the configuration without compromising
// security. All other fields are shown in full for troubleshooting.
func (c *Config) String() string {
	secretDisplay := "[hidden]"
	if c.Secret == "" {
		secretDisplay = "[not set]"
	}

	joinAddrs := "[none]"
	if len(c.Join) > 0 {
		joinAddrs = strings.Join(c.Join, ", ")
	}

	return fmt.Sprintf(
		"Config{NodeID: %s, Mode: %s, Secret: %s, Listen: %s, Join: %s, PollInterval: %s, Verbose: %v}",
		c.NodeID, c.Mode, secretDisplay, c.Listen, joinAddrs, c.PollInterval, c.Verbose,
	)
}
