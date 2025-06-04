//go:build integration
// +build integration

package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yourusername/linkpearl/pkg/clipboard"
	"github.com/yourusername/linkpearl/pkg/config"
	"github.com/yourusername/linkpearl/pkg/mesh"
	"github.com/yourusername/linkpearl/pkg/sync"
	"github.com/yourusername/linkpearl/pkg/transport"
)

// TestEndToEndClipboardSync tests the complete clipboard synchronization workflow
// between multiple nodes in various network configurations.
func TestEndToEndClipboardSync(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{"TwoNodeSync", testTwoNodeSync},
		{"ThreeNodeMesh", testThreeNodeMesh},
		{"ClientServerTopology", testClientServerTopology},
		{"NodeFailureRecovery", testNodeFailureRecovery},
		{"ConcurrentClipboardChanges", testConcurrentClipboardChanges},
		{"LargeClipboardContent", testLargeClipboardContent},
		{"RapidClipboardChanges", testRapidClipboardChanges},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

// testTwoNodeSync tests basic clipboard synchronization between two nodes
func testTwoNodeSync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two nodes with mock clipboards
	node1, cleanup1 := createTestNode(t, "node1", ":0", nil)
	defer cleanup1()

	node2, cleanup2 := createTestNode(t, "node2", ":0", []string{node1.listenAddr})
	defer cleanup2()

	// Start both nodes
	if err := node1.Start(ctx); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(ctx); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}

	// Wait for connection establishment
	waitForPeerConnection(t, node1, "node2", 5*time.Second)
	waitForPeerConnection(t, node2, "node1", 5*time.Second)

	// Test clipboard sync from node1 to node2
	testContent := "Hello from node1!"
	node1.clipboard.Write(testContent)

	// Verify content appears on node2
	assertClipboardContent(t, node2.clipboard, testContent, 5*time.Second)

	// Test sync in reverse direction
	testContent2 := "Hello from node2!"
	node2.clipboard.Write(testContent2)

	// Verify content appears on node1
	assertClipboardContent(t, node1.clipboard, testContent2, 5*time.Second)
}

// testThreeNodeMesh tests clipboard sync in a fully connected mesh topology
func testThreeNodeMesh(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create three nodes in a mesh
	node1, cleanup1 := createTestNode(t, "node1", ":0", nil)
	defer cleanup1()

	node2, cleanup2 := createTestNode(t, "node2", ":0", []string{node1.listenAddr})
	defer cleanup2()

	node3, cleanup3 := createTestNode(t, "node3", ":0", []string{node1.listenAddr, node2.listenAddr})
	defer cleanup3()

	// Start all nodes
	nodes := []*testNode{node1, node2, node3}
	for _, node := range nodes {
		if err := node.Start(ctx); err != nil {
			t.Fatalf("Failed to start %s: %v", node.id, err)
		}
	}

	// Wait for mesh formation
	waitForMeshFormation(t, nodes, 10*time.Second)

	// Test clipboard propagation from node1 to all nodes
	testContent := "Broadcast from node1"
	node1.clipboard.Write(testContent)

	// Verify all nodes receive the content
	for _, node := range nodes {
		if node != node1 {
			assertClipboardContent(t, node.clipboard, testContent, 5*time.Second)
		}
	}

	// Test from node3 to verify full mesh
	testContent2 := "Message from node3"
	node3.clipboard.Write(testContent2)

	// Verify all nodes receive the content
	for _, node := range nodes {
		if node != node3 {
			assertClipboardContent(t, node.clipboard, testContent2, 5*time.Second)
		}
	}
}

// testClientServerTopology tests a hub-and-spoke topology with client nodes
func testClientServerTopology(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create one server (full node) and two clients
	server, cleanupServer := createTestNode(t, "server", ":0", nil)
	defer cleanupServer()

	client1, cleanupClient1 := createTestNode(t, "client1", "", []string{server.listenAddr})
	defer cleanupClient1()
	client1.mode = "client" // Client mode - no listening

	client2, cleanupClient2 := createTestNode(t, "client2", "", []string{server.listenAddr})
	defer cleanupClient2()
	client2.mode = "client" // Client mode - no listening

	// Start all nodes
	nodes := []*testNode{server, client1, client2}
	for _, node := range nodes {
		if err := node.Start(ctx); err != nil {
			t.Fatalf("Failed to start %s: %v", node.id, err)
		}
	}

	// Wait for connections
	waitForPeerConnection(t, server, "client1", 5*time.Second)
	waitForPeerConnection(t, server, "client2", 5*time.Second)

	// Test clipboard sync through server
	testContent := "From client1 via server"
	client1.clipboard.Write(testContent)

	// Verify server and client2 receive the content
	assertClipboardContent(t, server.clipboard, testContent, 5*time.Second)
	assertClipboardContent(t, client2.clipboard, testContent, 5*time.Second)
}

// testNodeFailureRecovery tests reconnection and sync after node failure
func testNodeFailureRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create two nodes
	node1, cleanup1 := createTestNode(t, "node1", ":0", nil)
	defer cleanup1()

	node2, cleanup2 := createTestNode(t, "node2", ":0", []string{node1.listenAddr})
	defer cleanup2()

	// Start both nodes
	if err := node1.Start(ctx); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(ctx); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}

	// Wait for initial connection
	waitForPeerConnection(t, node1, "node2", 5*time.Second)

	// Simulate node2 failure
	node2.Stop()

	// Wait for disconnection to be detected
	waitForPeerDisconnection(t, node1, "node2", 5*time.Second)

	// Update clipboard on node1 while node2 is down
	offlineContent := "Changed while node2 was offline"
	node1.clipboard.Write(offlineContent)

	// Restart node2
	if err := node2.Start(ctx); err != nil {
		t.Fatalf("Failed to restart node2: %v", err)
	}

	// Wait for reconnection
	waitForPeerConnection(t, node1, "node2", 30*time.Second)

	// Verify node2 receives the update after reconnection
	assertClipboardContent(t, node2.clipboard, offlineContent, 10*time.Second)
}

// testConcurrentClipboardChanges tests conflict resolution with simultaneous changes
func testConcurrentClipboardChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create three nodes
	nodes := make([]*testNode, 3)
	cleanups := make([]func(), 3)

	nodes[0], cleanups[0] = createTestNode(t, "node0", ":0", nil)
	defer cleanups[0]()

	for i := 1; i < 3; i++ {
		nodes[i], cleanups[i] = createTestNode(t, fmt.Sprintf("node%d", i), ":0", []string{nodes[0].listenAddr})
		defer cleanups[i]()
	}

	// Start all nodes
	for _, node := range nodes {
		if err := node.Start(ctx); err != nil {
			t.Fatalf("Failed to start %s: %v", node.id, err)
		}
	}

	// Wait for mesh formation
	waitForMeshFormation(t, nodes, 10*time.Second)

	// Simulate concurrent clipboard changes
	done := make(chan struct{})
	for i, node := range nodes {
		go func(idx int, n *testNode) {
			content := fmt.Sprintf("Concurrent update from node%d", idx)
			n.clipboard.Write(content)
			done <- struct{}{}
		}(i, node)
	}

	// Wait for all updates to complete
	for i := 0; i < len(nodes); i++ {
		<-done
	}

	// Wait for convergence
	time.Sleep(3 * time.Second)

	// Verify all nodes have the same content (last-write-wins)
	contents := make([]string, len(nodes))
	for i, node := range nodes {
		content, err := node.clipboard.Read()
		if err != nil {
			t.Fatalf("Failed to read clipboard from %s: %v", node.id, err)
		}
		contents[i] = content
	}

	// All nodes should have converged to the same value
	for i := 1; i < len(contents); i++ {
		if contents[i] != contents[0] {
			t.Errorf("Nodes did not converge: node0=%q, node%d=%q", contents[0], i, contents[i])
		}
	}
}

// testLargeClipboardContent tests synchronization of large clipboard content
func testLargeClipboardContent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two nodes
	node1, cleanup1 := createTestNode(t, "node1", ":0", nil)
	defer cleanup1()

	node2, cleanup2 := createTestNode(t, "node2", ":0", []string{node1.listenAddr})
	defer cleanup2()

	// Start both nodes
	if err := node1.Start(ctx); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(ctx); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}

	// Wait for connection
	waitForPeerConnection(t, node1, "node2", 5*time.Second)

	// Create large content (100KB)
	largeContent := generateLargeContent(100 * 1024)
	node1.clipboard.Write(largeContent)

	// Verify sync of large content
	assertClipboardContent(t, node2.clipboard, largeContent, 10*time.Second)
}

// testRapidClipboardChanges tests handling of rapid clipboard updates
func testRapidClipboardChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two nodes
	node1, cleanup1 := createTestNode(t, "node1", ":0", nil)
	defer cleanup1()

	node2, cleanup2 := createTestNode(t, "node2", ":0", []string{node1.listenAddr})
	defer cleanup2()

	// Start both nodes
	if err := node1.Start(ctx); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(ctx); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}

	// Wait for connection
	waitForPeerConnection(t, node1, "node2", 5*time.Second)

	// Rapidly update clipboard
	updates := 10
	for i := 0; i < updates; i++ {
		content := fmt.Sprintf("Rapid update %d", i)
		node1.clipboard.Write(content)
		time.Sleep(100 * time.Millisecond)
	}

	// Verify final state is synchronized
	finalContent := fmt.Sprintf("Rapid update %d", updates-1)
	assertClipboardContent(t, node2.clipboard, finalContent, 5*time.Second)
}

// Helper types and functions

type testNode struct {
	id         string
	mode       string
	listenAddr string
	joinAddrs  []string
	clipboard  *clipboard.MockClipboard
	transport  transport.Transport
	topology   *mesh.Topology
	engine     *sync.Engine
	cancel     context.CancelFunc
}

func (n *testNode) Start(ctx context.Context) error {
	nodeCtx, cancel := context.WithCancel(ctx)
	n.cancel = cancel

	// Start transport if listening
	if n.listenAddr != "" && n.listenAddr != ":0" {
		if err := n.transport.Listen(n.listenAddr); err != nil {
			return fmt.Errorf("transport listen failed: %w", err)
		}
		// Update listen address if using :0
		if n.listenAddr == ":0" {
			n.listenAddr = n.transport.Addr().String()
		}
	}

	// Start topology
	if err := n.topology.Start(nodeCtx); err != nil {
		return fmt.Errorf("topology start failed: %w", err)
	}

	// Start sync engine
	go func() {
		if err := n.engine.Run(nodeCtx); err != nil && err != context.Canceled {
			// Log error but don't fail the test
			fmt.Printf("Sync engine error for %s: %v\n", n.id, err)
		}
	}()

	return nil
}

func (n *testNode) Stop() {
	if n.cancel != nil {
		n.cancel()
	}
	n.topology.Stop()
	n.transport.Close()
}

func createTestNode(t *testing.T, nodeID, listenAddr string, joinAddrs []string) (*testNode, func()) {
	// Create mock clipboard
	mockClip := clipboard.NewMockClipboard()

	// Create transport with test secret
	auth := &transport.DefaultAuthenticator{}
	trans := &transport.TCPTransport{
		Secret: "test-secret",
		Auth:   auth,
	}

	// Determine node mode
	mode := "full"
	if listenAddr == "" {
		mode = "client"
	}

	// Create topology
	topo := &mesh.Topology{
		NodeID:    nodeID,
		Mode:      mode,
		Transport: trans,
		JoinAddrs: joinAddrs,
	}

	// Create sync engine
	engine := &sync.Engine{
		NodeID:    nodeID,
		Clipboard: mockClip,
		Topology:  topo,
	}

	node := &testNode{
		id:         nodeID,
		mode:       mode,
		listenAddr: listenAddr,
		joinAddrs:  joinAddrs,
		clipboard:  mockClip,
		transport:  trans,
		topology:   topo,
		engine:     engine,
	}

	cleanup := func() {
		node.Stop()
	}

	return node, cleanup
}

func waitForPeerConnection(t *testing.T, node *testNode, peerID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if node.topology.HasPeer(peerID) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for %s to connect to %s", node.id, peerID)
}

func waitForPeerDisconnection(t *testing.T, node *testNode, peerID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !node.topology.HasPeer(peerID) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for %s to disconnect from %s", node.id, peerID)
}

func waitForMeshFormation(t *testing.T, nodes []*testNode, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allConnected := true
		for _, node := range nodes {
			for _, peer := range nodes {
				if node.id != peer.id && node.mode == "full" {
					if !node.topology.HasPeer(peer.id) {
						allConnected = false
						break
					}
				}
			}
			if !allConnected {
				break
			}
		}
		if allConnected {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Timeout waiting for mesh formation")
}

func assertClipboardContent(t *testing.T, clip clipboard.Clipboard, expected string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		content, err := clip.Read()
		if err == nil && content == expected {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	// Final check with actual values for better error message
	actual, err := clip.Read()
	if err != nil {
		t.Fatalf("Failed to read clipboard: %v", err)
	}
	t.Fatalf("Clipboard content mismatch: expected %q, got %q", expected, actual)
}

func generateLargeContent(size int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, size)
	for i := range b {
		b[i] = chars[i%len(chars)]
	}
	return string(b)
}

// Benchmark tests

func BenchmarkTwoNodeSync(b *testing.B) {
	ctx := context.Background()

	// Create two nodes
	node1, cleanup1 := createTestNode(b, "node1", ":0", nil)
	defer cleanup1()

	node2, cleanup2 := createTestNode(b, "node2", ":0", []string{node1.listenAddr})
	defer cleanup2()

	// Start nodes
	if err := node1.Start(ctx); err != nil {
		b.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(ctx); err != nil {
		b.Fatalf("Failed to start node2: %v", err)
	}

	// Wait for connection
	waitForPeerConnection(b, node1, "node2", 5*time.Second)

	b.ResetTimer()

	// Benchmark clipboard sync operations
	for i := 0; i < b.N; i++ {
		content := fmt.Sprintf("Benchmark content %d", i)
		node1.clipboard.Write(content)
		
		// Wait for sync
		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			if c, _ := node2.clipboard.Read(); c == content {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func BenchmarkLargeContentSync(b *testing.B) {
	ctx := context.Background()

	// Create two nodes
	node1, cleanup1 := createTestNode(b, "node1", ":0", nil)
	defer cleanup1()

	node2, cleanup2 := createTestNode(b, "node2", ":0", []string{node1.listenAddr})
	defer cleanup2()

	// Start nodes
	if err := node1.Start(ctx); err != nil {
		b.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(ctx); err != nil {
		b.Fatalf("Failed to start node2: %v", err)
	}

	// Wait for connection
	waitForPeerConnection(b, node1, "node2", 5*time.Second)

	// Generate test content of various sizes
	sizes := []int{1024, 10240, 102400} // 1KB, 10KB, 100KB

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dKB", size/1024), func(b *testing.B) {
			content := generateLargeContent(size)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				node1.clipboard.Write(content)
				
				// Wait for sync
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					if c, _ := node2.clipboard.Read(); c == content {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		})
	}
}

// Helper function for TestMain - allows running integration tests with custom setup
func TestMain(m *testing.M) {
	// You can add any global setup here if needed
	// For example, checking if required tools are available
	
	// Run tests
	m.Run()
	
	// Global cleanup if needed
}