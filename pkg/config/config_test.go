package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	if cfg.NodeID == "" {
		t.Error("NewConfig should generate a node ID")
	}

	if cfg.Mode != ClientNode {
		t.Errorf("Default mode should be ClientNode, got %s", cfg.Mode)
	}

	if cfg.PollInterval != 500*time.Millisecond {
		t.Errorf("Default poll interval should be 500ms, got %s", cfg.PollInterval)
	}

	if cfg.ReconnectBackoff != time.Second {
		t.Errorf("Default reconnect backoff should be 1s, got %s", cfg.ReconnectBackoff)
	}

	if cfg.Verbose {
		t.Error("Default verbose should be false")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid client config",
			config: &Config{
				Secret: "mysecret",
				NodeID: "test-node",
				Mode:   ClientNode,
				Join:   []string{"localhost:8080"},
			},
			wantErr: false,
		},
		{
			name: "valid full node config",
			config: &Config{
				Secret: "mysecret",
				NodeID: "test-node",
				Mode:   FullNode,
				Listen: ":8080",
			},
			wantErr: false,
		},
		{
			name: "listen address sets mode to full",
			config: &Config{
				Secret: "mysecret",
				NodeID: "test-node",
				Mode:   ClientNode,
				Listen: ":8080",
			},
			wantErr: false,
		},
		{
			name: "missing secret",
			config: &Config{
				NodeID: "test-node",
				Mode:   ClientNode,
				Join:   []string{"localhost:8080"},
			},
			wantErr: true,
			errMsg:  "secret is required",
		},
		{
			name: "missing node ID",
			config: &Config{
				Secret: "mysecret",
				Mode:   ClientNode,
				Join:   []string{"localhost:8080"},
			},
			wantErr: true,
			errMsg:  "node ID is required",
		},
		{
			name: "client without join addresses",
			config: &Config{
				Secret: "mysecret",
				NodeID: "test-node",
				Mode:   ClientNode,
			},
			wantErr: true,
			errMsg:  "client nodes must specify at least one address to join",
		},
		{
			name: "full node without listen address",
			config: &Config{
				Secret: "mysecret",
				NodeID: "test-node",
				Mode:   FullNode,
			},
			wantErr: true,
			errMsg:  "full nodes must specify a listen address",
		},
		{
			name: "client with listen address",
			config: &Config{
				Secret: "mysecret",
				NodeID: "test-node",
				Mode:   ClientNode,
				Listen: ":8080",
				Join:   []string{"localhost:9090"},
			},
			wantErr: false, // Should auto-correct to FullNode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
			}

			// Check mode auto-correction
			if tt.name == "listen address sets mode to full" && tt.config.Mode != FullNode {
				t.Errorf("Mode should be auto-corrected to FullNode when Listen is set")
			}
		})
	}
}

func TestConfigLoadFromEnv(t *testing.T) {
	// Save current env and restore after test
	origEnv := map[string]string{
		"LINKPEARL_SECRET":            os.Getenv("LINKPEARL_SECRET"),
		"LINKPEARL_NODE_ID":           os.Getenv("LINKPEARL_NODE_ID"),
		"LINKPEARL_MODE":              os.Getenv("LINKPEARL_MODE"),
		"LINKPEARL_LISTEN":            os.Getenv("LINKPEARL_LISTEN"),
		"LINKPEARL_JOIN":              os.Getenv("LINKPEARL_JOIN"),
		"LINKPEARL_POLL_INTERVAL":     os.Getenv("LINKPEARL_POLL_INTERVAL"),
		"LINKPEARL_RECONNECT_BACKOFF": os.Getenv("LINKPEARL_RECONNECT_BACKOFF"),
		"LINKPEARL_VERBOSE":           os.Getenv("LINKPEARL_VERBOSE"),
	}
	defer func() {
		for k, v := range origEnv {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Set test environment
	os.Setenv("LINKPEARL_SECRET", "envsecret")
	os.Setenv("LINKPEARL_NODE_ID", "env-node")
	os.Setenv("LINKPEARL_MODE", "full")
	os.Setenv("LINKPEARL_LISTEN", ":9090")
	os.Setenv("LINKPEARL_JOIN", "host1:8080, host2:8080")
	os.Setenv("LINKPEARL_POLL_INTERVAL", "2s")
	os.Setenv("LINKPEARL_RECONNECT_BACKOFF", "5s")
	os.Setenv("LINKPEARL_VERBOSE", "true")

	cfg := NewConfig()
	cfg.LoadFromEnv()

	if cfg.Secret != "envsecret" {
		t.Errorf("Secret = %s, want envsecret", cfg.Secret)
	}

	if cfg.NodeID != "env-node" {
		t.Errorf("NodeID = %s, want env-node", cfg.NodeID)
	}

	if cfg.Mode != FullNode {
		t.Errorf("Mode = %s, want full", cfg.Mode)
	}

	if cfg.Listen != ":9090" {
		t.Errorf("Listen = %s, want :9090", cfg.Listen)
	}

	if len(cfg.Join) != 2 || cfg.Join[0] != "host1:8080" || cfg.Join[1] != "host2:8080" {
		t.Errorf("Join = %v, want [host1:8080 host2:8080]", cfg.Join)
	}

	if cfg.PollInterval != 2*time.Second {
		t.Errorf("PollInterval = %s, want 2s", cfg.PollInterval)
	}

	if cfg.ReconnectBackoff != 5*time.Second {
		t.Errorf("ReconnectBackoff = %s, want 5s", cfg.ReconnectBackoff)
	}

	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
}

func TestConfigString(t *testing.T) {
	cfg := &Config{
		Secret: "mysecret",
		NodeID: "test-node",
		Mode:   FullNode,
		Listen: ":8080",
		Join:   []string{"host1:8080", "host2:8080"},
	}

	str := cfg.String()

	// Should hide secret
	if strings.Contains(str, "mysecret") {
		t.Error("String() should not expose the secret")
	}

	if !strings.Contains(str, "[hidden]") {
		t.Error("String() should show [hidden] for secret")
	}

	// Should include other fields
	if !strings.Contains(str, "test-node") {
		t.Error("String() should include NodeID")
	}

	if !strings.Contains(str, "full") {
		t.Error("String() should include Mode")
	}

	if !strings.Contains(str, ":8080") {
		t.Error("String() should include Listen")
	}

	// Test with empty secret
	cfg.Secret = ""
	str = cfg.String()
	if !strings.Contains(str, "[not set]") {
		t.Error("String() should show [not set] for empty secret")
	}
}

func TestGenerateNodeID(t *testing.T) {
	id1 := generateNodeID()
	id2 := generateNodeID()

	if id1 == "" {
		t.Error("generateNodeID should not return empty string")
	}

	// IDs should be unique (due to timestamp)
	if id1 == id2 {
		t.Error("generateNodeID should generate unique IDs")
	}

	// Should contain hostname
	hostname, _ := os.Hostname()
	if hostname != "" && !strings.Contains(id1, hostname) {
		t.Errorf("NodeID should contain hostname, got %s", id1)
	}
}
