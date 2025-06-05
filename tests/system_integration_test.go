//go:build integration && !ci
// +build integration,!ci

package tests

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Veraticus/linkpearl/pkg/clipboard"
	"github.com/Veraticus/linkpearl/pkg/config"
	"github.com/Veraticus/linkpearl/pkg/mesh"
	"github.com/Veraticus/linkpearl/pkg/sync"
	"github.com/Veraticus/linkpearl/pkg/transport"
)

// TestSystemClipboardIntegration tests with real system clipboards
// This test is skipped in CI environments where clipboard access may not be available
func TestSystemClipboardIntegration(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping system clipboard test in CI environment")
	}

	// Check if we can access the system clipboard
	clip, err := clipboard.NewPlatformClipboard()
	if err != nil {
		t.Skipf("System clipboard not available: %v", err)
	}

	// Save original clipboard content
	originalContent, _ := clip.Read()
	defer func() {
		// Restore original clipboard content
		_ = clip.Write(originalContent)
	}()

	t.Run("LocalClipboardWatch", testLocalClipboardWatch)
	t.Run("SystemClipboardSync", testSystemClipboardSync)
}

// testLocalClipboardWatch tests the clipboard watch functionality with real clipboard
func testLocalClipboardWatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clip, err := clipboard.NewPlatformClipboard()
	if err != nil {
		t.Skipf("System clipboard not available: %v", err)
	}

	// Start watching clipboard
	watchCh := clip.Watch(ctx)

	// Write test content
	testContent := fmt.Sprintf("Test content %d", time.Now().Unix())
	if err := clip.Write(testContent); err != nil {
		t.Fatalf("Failed to write to clipboard: %v", err)
	}

	// Wait for change notification
	select {
	case <-watchCh:
		// Notification received, now read the content
		content, err := clip.Read()
		if err != nil {
			t.Fatalf("Failed to read clipboard after notification: %v", err)
		}
		if content != testContent {
			t.Errorf("Expected %q, got %q", testContent, content)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for clipboard change notification")
	}
}

// testSystemClipboardSync tests end-to-end sync with real system clipboards
func testSystemClipboardSync(t *testing.T) {
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("No display available for clipboard access")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create configuration
	cfg1 := &config.Config{
		Secret:       "integration-test-secret",
		NodeID:       "test-node-1",
		Listen:       ":0",
		PollInterval: 500 * time.Millisecond,
	}

	cfg2 := &config.Config{
		Secret:       "integration-test-secret",
		NodeID:       "test-node-2",
		Listen:       ":0",
		PollInterval: 500 * time.Millisecond,
	}

	// Create nodes with real clipboard
	node1, cleanup1 := createSystemTestNode(t, cfg1)
	defer cleanup1()

	// Get node1's listen address after it starts
	if err := node1.Start(ctx); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}

	cfg2.Join = []string{node1.transport.Addr().String()}
	node2, cleanup2 := createSystemTestNode(t, cfg2)
	defer cleanup2()

	if err := node2.Start(ctx); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}

	// Wait for connection
	waitForSystemNodeConnection(t, node1, node2, 10*time.Second)

	// Test 1: Write to system clipboard and verify sync
	testContent1 := fmt.Sprintf("Integration test content %d", time.Now().Unix())
	if err := node1.clipboard.Write(testContent1); err != nil {
		t.Fatalf("Failed to write to clipboard: %v", err)
	}

	// Verify content syncs to node2's clipboard
	assertSystemClipboardContent(t, node2.clipboard, testContent1, 10*time.Second)

	// Test 2: Write to node2 and verify sync back
	testContent2 := fmt.Sprintf("Reverse sync test %d", time.Now().Unix())
	if err := node2.clipboard.Write(testContent2); err != nil {
		t.Fatalf("Failed to write to clipboard: %v", err)
	}

	// Verify content syncs to node1's clipboard
	assertSystemClipboardContent(t, node1.clipboard, testContent2, 10*time.Second)

	// Test 3: Rapid updates
	for i := 0; i < 5; i++ {
		content := fmt.Sprintf("Rapid update %d at %d", i, time.Now().Unix())
		if err := node1.clipboard.Write(content); err != nil {
			t.Fatalf("Failed to write rapid update: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Verify final state is consistent
	finalContent, err := node1.clipboard.Read()
	if err != nil {
		t.Fatalf("Failed to read final content: %v", err)
	}
	assertSystemClipboardContent(t, node2.clipboard, finalContent, 10*time.Second)
}

// testPerformanceWithSystemClipboard measures real-world performance
func TestPerformanceWithSystemClipboard(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping system clipboard test in CI environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create two nodes with system clipboards
	cfg1 := &config.Config{
		Secret:       "perf-test-secret",
		NodeID:       "perf-node-1",
		Listen:       ":0",
		PollInterval: 200 * time.Millisecond, // Faster polling for performance test
	}

	node1, cleanup1 := createSystemTestNode(t, cfg1)
	defer cleanup1()

	if err := node1.Start(ctx); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}

	cfg2 := &config.Config{
		Secret:       "perf-test-secret",
		NodeID:       "perf-node-2",
		Listen:       ":0",
		Join:         []string{node1.transport.Addr().String()},
		PollInterval: 200 * time.Millisecond,
	}

	node2, cleanup2 := createSystemTestNode(t, cfg2)
	defer cleanup2()

	if err := node2.Start(ctx); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}

	// Wait for connection
	waitForSystemNodeConnection(t, node1, node2, 10*time.Second)

	// Measure sync latency
	latencies := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		content := fmt.Sprintf("Latency test %d", i)
		start := time.Now()

		if err := node1.clipboard.Write(content); err != nil {
			t.Fatalf("Failed to write: %v", err)
		}

		// Wait for sync
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if c, _ := node2.clipboard.Read(); c == content {
				latencies[i] = time.Since(start)
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if latencies[i] == 0 {
			t.Errorf("Sync timeout for iteration %d", i)
			latencies[i] = 5 * time.Second
		}

		time.Sleep(500 * time.Millisecond) // Wait between tests
	}

	// Calculate statistics
	var total time.Duration
	var min, max time.Duration
	for i, lat := range latencies {
		total += lat
		if i == 0 || lat < min {
			min = lat
		}
		if lat > max {
			max = lat
		}
	}
	avg := total / time.Duration(len(latencies))

	t.Logf("Sync latency - Min: %v, Max: %v, Avg: %v", min, max, avg)

	// Performance assertions
	if avg > 2*time.Second {
		t.Errorf("Average sync latency too high: %v", avg)
	}
	if max > 3*time.Second {
		t.Errorf("Maximum sync latency too high: %v", max)
	}
}

// Helper types for system tests

type systemTestNode struct {
	id        string
	config    *config.Config
	clipboard clipboard.Clipboard
	transport transport.Transport
	topology  mesh.Topology
	engine    sync.Engine
	cancel    context.CancelFunc
}

func (n *systemTestNode) Start(ctx context.Context) error {
	nodeCtx, cancel := context.WithCancel(ctx)
	n.cancel = cancel

	// Start topology (which handles transport listening)
	if err := n.topology.Start(nodeCtx); err != nil {
		return fmt.Errorf("topology start failed: %w", err)
	}

	// Start sync engine
	go func() {
		if err := n.engine.Run(nodeCtx); err != nil && err != context.Canceled {
			fmt.Printf("Sync engine error for %s: %v\n", n.id, err)
		}
	}()

	return nil
}

func (n *systemTestNode) Stop() {
	if n.cancel != nil {
		n.cancel()
	}
	if n.topology != nil {
		_ = n.topology.Stop()
	}
}

func createSystemTestNode(t *testing.T, cfg *config.Config) (*systemTestNode, func()) {
	// Create real clipboard
	clip, err := clipboard.NewPlatformClipboard()
	if err != nil {
		t.Skipf("Failed to create clipboard: %v", err)
	}

	// Determine node mode
	mode := "full"
	if cfg.Listen == "" {
		mode = "client"
	}

	// Create transport config
	transportConfig := &transport.TransportConfig{
		NodeID: cfg.NodeID,
		Mode:   mode,
		Secret: cfg.Secret,
		Logger: transport.DefaultLogger(),
	}

	// Create transport
	trans := transport.NewTCPTransport(transportConfig)

	// Create topology config
	topoConfig := mesh.DefaultTopologyConfig()
	topoConfig.Self = mesh.Node{
		ID:   cfg.NodeID,
		Mode: mode,
		Addr: cfg.Listen,
	}
	topoConfig.Transport = trans
	topoConfig.JoinAddrs = cfg.Join

	// Create topology
	topo, err := mesh.NewTopology(topoConfig)
	if err != nil {
		t.Fatalf("Failed to create topology for %s: %v", cfg.NodeID, err)
	}

	// Create sync engine config
	syncConfig := &sync.Config{
		NodeID:    cfg.NodeID,
		Clipboard: clip,
		Topology:  topo,
	}

	// Create sync engine
	engine, err := sync.NewEngine(syncConfig)
	if err != nil {
		t.Fatalf("Failed to create sync engine for %s: %v", cfg.NodeID, err)
	}

	node := &systemTestNode{
		id:        cfg.NodeID,
		config:    cfg,
		clipboard: clip,
		transport: trans,
		topology:  topo,
		engine:    engine,
	}

	cleanup := func() {
		node.Stop()
	}

	return node, cleanup
}

func waitForSystemNodeConnection(t *testing.T, node1, node2 *systemTestNode, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err1 := node1.topology.GetPeer(node2.id); err1 == nil {
			if _, err2 := node2.topology.GetPeer(node1.id); err2 == nil {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for nodes to connect")
}

func assertSystemClipboardContent(t *testing.T, clip clipboard.Clipboard, expected string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	var lastContent string
	var lastErr error

	for time.Now().Before(deadline) {
		content, err := clip.Read()
		lastContent = content
		lastErr = err

		if err == nil && content == expected {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("Failed to read clipboard: %v", lastErr)
	}
	t.Fatalf("Clipboard content mismatch: expected %q, got %q", expected, lastContent)
}
