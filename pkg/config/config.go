package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// NodeMode represents the operational mode of a linkpearl node
type NodeMode string

const (
	// ClientNode connects to other nodes but doesn't accept incoming connections
	ClientNode NodeMode = "client"
	// FullNode accepts incoming connections and participates fully in the mesh
	FullNode NodeMode = "full"
)

// Config holds all configuration for linkpearl
type Config struct {
	// Core settings
	Secret string   `env:"LINKPEARL_SECRET"`
	NodeID string   `env:"LINKPEARL_NODE_ID"`
	Mode   NodeMode `env:"LINKPEARL_MODE"`

	// Network settings
	Listen string   `env:"LINKPEARL_LISTEN"`
	Join   []string `env:"LINKPEARL_JOIN"`

	// Behavior
	PollInterval     time.Duration `env:"LINKPEARL_POLL_INTERVAL"`
	ReconnectBackoff time.Duration `env:"LINKPEARL_RECONNECT_BACKOFF"`
	Verbose          bool          `env:"LINKPEARL_VERBOSE"`
}

// NewConfig creates a new config with sensible defaults
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

// Validate ensures the configuration is valid
func (c *Config) Validate() error {
	if c.Secret == "" {
		return fmt.Errorf("secret is required")
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

// LoadFromEnv loads configuration from environment variables
func (c *Config) LoadFromEnv() {
	if secret := os.Getenv("LINKPEARL_SECRET"); secret != "" {
		c.Secret = secret
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

// generateNodeID creates a unique node identifier
func generateNodeID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	// Use UnixNano for more uniqueness
	return fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
}

// String returns a string representation of the config (hiding sensitive data)
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