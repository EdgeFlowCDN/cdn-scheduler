package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNodeHealthLoadScore(t *testing.T) {
	n := &NodeHealth{
		CPUUsage:       0.5,
		MemUsage:       0.3,
		BandwidthUsed:  5000,
		BandwidthMax:   10000,
		Connections:    500,
		MaxConnections: 10000,
	}

	score := n.LoadScore()
	// 0.5*0.3 + 0.3*0.2 + 0.5*0.4 + 0.05*0.1 = 0.15 + 0.06 + 0.2 + 0.005 = 0.415
	expected := 0.415
	if score < expected-0.01 || score > expected+0.01 {
		t.Errorf("LoadScore() = %f, expected ~%f", score, expected)
	}
}

func TestCheckerHealthyNodes(t *testing.T) {
	// Start a mock health endpoint
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()

	unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthy.Close()

	checker := NewChecker(100*time.Millisecond, 1*time.Second, 2, 1)
	checker.RegisterNode("node1", "127.0.0.1", 1000, 100, healthy.URL)
	checker.RegisterNode("node2", "127.0.0.2", 1000, 100, unhealthy.URL)

	// Run checks manually
	checker.checkAll()
	time.Sleep(50 * time.Millisecond)

	nodes := checker.GetHealthyNodes()
	if len(nodes) != 1 {
		t.Errorf("expected 1 healthy node, got %d", len(nodes))
	}
	if len(nodes) > 0 && nodes[0].Name != "node1" {
		t.Errorf("healthy node = %s, want node1", nodes[0].Name)
	}
}

func TestCheckerMaintenanceMode(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()

	checker := NewChecker(100*time.Millisecond, 1*time.Second, 2, 1)
	checker.RegisterNode("node1", "127.0.0.1", 1000, 100, healthy.URL)

	checker.checkAll()
	time.Sleep(50 * time.Millisecond)

	// Node should be online
	if n := checker.GetNode("node1"); n.Status != StatusOnline {
		t.Errorf("status = %s, want online", n.Status)
	}

	// Set maintenance
	checker.SetNodeStatus("node1", StatusMaintenance)
	nodes := checker.GetHealthyNodes()
	if len(nodes) != 0 {
		t.Error("maintenance node should not be in healthy list")
	}
}

func TestCheckerFailureThreshold(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()

	checker := NewChecker(50*time.Millisecond, 1*time.Second, 3, 1)
	checker.RegisterNode("node1", "127.0.0.1", 1000, 100, failing.URL)

	// Force online first
	checker.mu.Lock()
	checker.nodes["node1"].Status = StatusOnline
	checker.mu.Unlock()

	// 2 failures should not take it offline (threshold is 3)
	checker.checkAll()
	checker.checkAll()
	if n := checker.GetNode("node1"); n.Status != StatusOnline {
		t.Error("should still be online after 2 failures")
	}

	// 3rd failure should take it offline
	checker.checkAll()
	if n := checker.GetNode("node1"); n.Status != StatusOffline {
		t.Errorf("should be offline after 3 failures, got %s", n.Status)
	}
}
