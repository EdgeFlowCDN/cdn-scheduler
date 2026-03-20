package health

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Status represents the health state of a node.
type Status string

const (
	StatusOnline      Status = "online"
	StatusOffline     Status = "offline"
	StatusMaintenance Status = "maintenance"
)

// NodeHealth tracks the health status of a node.
type NodeHealth struct {
	Name             string
	IP               string
	Status           Status
	ConsecFails      int
	ConsecSuccesses  int
	LastCheck        time.Time
	LastHeartbeat    time.Time
	CPUUsage         float64
	MemUsage         float64
	BandwidthUsed    int64
	BandwidthMax     int64
	Connections      int64
	MaxConnections   int64
}

// LoadScore calculates the load score for a node (0-1, lower is better).
func (n *NodeHealth) LoadScore() float64 {
	score := 0.0
	score += n.CPUUsage * 0.3
	score += n.MemUsage * 0.2
	if n.BandwidthMax > 0 {
		score += (float64(n.BandwidthUsed) / float64(n.BandwidthMax)) * 0.4
	}
	if n.MaxConnections > 0 {
		score += (float64(n.Connections) / float64(n.MaxConnections)) * 0.1
	}
	return score
}

// Checker performs health checks on edge nodes.
type Checker struct {
	mu               sync.RWMutex
	nodes            map[string]*NodeHealth
	interval         time.Duration
	timeout          time.Duration
	failureThreshold int
	successThreshold int
	client           *http.Client
	endpoints        map[string]string // name -> health endpoint URL
	stopCh           chan struct{}
}

// NewChecker creates a new health checker.
func NewChecker(interval, timeout time.Duration, failureThreshold, successThreshold int) *Checker {
	return &Checker{
		nodes:            make(map[string]*NodeHealth),
		interval:         interval,
		timeout:          timeout,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		client:           &http.Client{Timeout: timeout},
		endpoints:        make(map[string]string),
		stopCh:           make(chan struct{}),
	}
}

// RegisterNode adds a node to the health checker.
func (c *Checker) RegisterNode(name, ip string, bandwidthMax, maxConns int64, healthEndpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nodes[name] = &NodeHealth{
		Name:           name,
		IP:             ip,
		Status:         StatusOffline,
		BandwidthMax:   bandwidthMax,
		MaxConnections: maxConns,
	}

	if healthEndpoint == "" {
		healthEndpoint = fmt.Sprintf("http://%s:8080/health", ip)
	}
	c.endpoints[name] = healthEndpoint
}

// GetNode returns the health info for a node.
func (c *Checker) GetNode(name string) *NodeHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if h, ok := c.nodes[name]; ok {
		copy := *h
		return &copy
	}
	return nil
}

// GetHealthyNodes returns all nodes with online status.
func (c *Checker) GetHealthyNodes() []*NodeHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var healthy []*NodeHealth
	for _, n := range c.nodes {
		if n.Status == StatusOnline {
			copy := *n
			healthy = append(healthy, &copy)
		}
	}
	return healthy
}

// SetNodeStatus manually sets a node's status (e.g., maintenance).
func (c *Checker) SetNodeStatus(name string, status Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.nodes[name]; ok {
		n.Status = status
	}
}

// Start begins periodic health checking.
func (c *Checker) Start() {
	go c.loop()
}

// Stop stops the health checker.
func (c *Checker) Stop() {
	close(c.stopCh)
}

func (c *Checker) loop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Initial check
	c.checkAll()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.checkAll()
		}
	}
}

func (c *Checker) checkAll() {
	c.mu.RLock()
	names := make([]string, 0, len(c.nodes))
	for name := range c.nodes {
		names = append(names, name)
	}
	c.mu.RUnlock()

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			c.checkNode(name)
		}(name)
	}
	wg.Wait()
}

func (c *Checker) checkNode(name string) {
	c.mu.RLock()
	endpoint, ok := c.endpoints[name]
	node := c.nodes[name]
	if !ok || node == nil || node.Status == StatusMaintenance {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		c.recordFailure(name)
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.recordFailure(name)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		c.recordSuccess(name)
	} else {
		c.recordFailure(name)
	}
}

func (c *Checker) recordSuccess(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, ok := c.nodes[name]
	if !ok {
		return
	}

	n.ConsecFails = 0
	n.ConsecSuccesses++
	n.LastCheck = time.Now()

	if n.Status == StatusOffline && n.ConsecSuccesses >= c.successThreshold {
		n.Status = StatusOnline
		log.Printf("[health] node %s is now online", name)
	}
}

func (c *Checker) recordFailure(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, ok := c.nodes[name]
	if !ok {
		return
	}

	n.ConsecSuccesses = 0
	n.ConsecFails++
	n.LastCheck = time.Now()

	if n.Status == StatusOnline && n.ConsecFails >= c.failureThreshold {
		n.Status = StatusOffline
		log.Printf("[health] node %s is now offline (consecutive failures: %d)", name, n.ConsecFails)
	}
}
