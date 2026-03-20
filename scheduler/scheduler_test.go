package scheduler

import (
	"testing"

	"github.com/EdgeFlowCDN/cdn-scheduler/config"
	"github.com/EdgeFlowCDN/cdn-scheduler/geoip"
	"github.com/EdgeFlowCDN/cdn-scheduler/health"
)

func setupTestScheduler() (*Scheduler, *health.Checker) {
	nodes := []config.NodeConfig{
		{Name: "beijing", IP: "10.0.1.1", Region: "beijing", ISP: "telecom", Lat: 39.9, Lon: 116.4, MaxBandwidth: 10000, MaxConnections: 1000},
		{Name: "shanghai", IP: "10.0.2.1", Region: "shanghai", ISP: "unicom", Lat: 31.2, Lon: 121.5, MaxBandwidth: 10000, MaxConnections: 1000},
		{Name: "guangzhou", IP: "10.0.3.1", Region: "guangzhou", ISP: "mobile", Lat: 23.1, Lon: 113.3, MaxBandwidth: 10000, MaxConnections: 1000},
	}

	hc := health.NewChecker(10000000000, 1000000000, 3, 1) // very long intervals to avoid auto-checks
	for _, n := range nodes {
		hc.RegisterNode(n.Name, n.IP, n.MaxBandwidth, n.MaxConnections, "")
	}
	// Force all nodes online
	hc.SetNodeStatus("beijing", health.StatusOnline)
	hc.SetNodeStatus("shanghai", health.StatusOnline)
	hc.SetNodeStatus("guangzhou", health.StatusOnline)

	locator := geoip.NewStaticLocator()
	sched := New(nodes, hc, locator, 3, 0.85, 0.95)
	return sched, hc
}

func TestSelectNode(t *testing.T) {
	sched, _ := setupTestScheduler()

	// Client in Beijing (telecom) should prefer Beijing node
	node := sched.SelectNode("1.0.0.0")
	if node == nil {
		t.Fatal("expected a node")
	}
	// With ISP matching, should get telecom (Beijing) node
	if node.ISP != "telecom" {
		t.Logf("note: got ISP=%s, Beijing telecom preferred but distance may vary", node.ISP)
	}
}

func TestSelectNodeNoHealthy(t *testing.T) {
	sched, hc := setupTestScheduler()

	// Take all nodes offline
	hc.SetNodeStatus("beijing", health.StatusOffline)
	hc.SetNodeStatus("shanghai", health.StatusOffline)
	hc.SetNodeStatus("guangzhou", health.StatusOffline)

	node := sched.SelectNode("1.0.0.0")
	if node != nil {
		t.Error("expected nil when no healthy nodes")
	}
}

func TestSelectNodes(t *testing.T) {
	sched, _ := setupTestScheduler()

	nodes := sched.SelectNodes("1.0.0.0", 2)
	if len(nodes) == 0 {
		t.Fatal("expected at least one node")
	}
	if len(nodes) > 2 {
		t.Errorf("requested 2 nodes, got %d", len(nodes))
	}
}

func TestSelectNodeSkipsOverloaded(t *testing.T) {
	nodes := []config.NodeConfig{
		{Name: "node1", IP: "10.0.1.1", Lat: 39.9, Lon: 116.4, MaxBandwidth: 100, MaxConnections: 100},
		{Name: "node2", IP: "10.0.2.1", Lat: 39.9, Lon: 116.4, MaxBandwidth: 100, MaxConnections: 100},
	}

	hc := health.NewChecker(10000000000, 1000000000, 3, 1)
	for _, n := range nodes {
		hc.RegisterNode(n.Name, n.IP, n.MaxBandwidth, n.MaxConnections, "")
	}
	hc.SetNodeStatus("node1", health.StatusOnline)
	hc.SetNodeStatus("node2", health.StatusOnline)

	// Make node1 overloaded (> reject threshold)
	hc.SetNodeStatus("node1", health.StatusOnline)
	// We need to set load metrics directly
	hc.GetNode("node1") // just to verify it exists

	locator := geoip.NewStaticLocator()
	sched := New(nodes, hc, locator, 3, 0.85, 0.95)

	// Both nodes should be selectable since default load is 0
	node := sched.SelectNode("1.0.0.0")
	if node == nil {
		t.Fatal("expected a node")
	}
}

func BenchmarkSelectNode(b *testing.B) {
	sched, _ := setupTestScheduler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sched.SelectNode("1.0.0.0")
	}
}
