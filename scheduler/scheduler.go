package scheduler

import (
	"math/rand"
	"sort"

	"github.com/EdgeFlowCDN/cdn-scheduler/config"
	"github.com/EdgeFlowCDN/cdn-scheduler/geoip"
	"github.com/EdgeFlowCDN/cdn-scheduler/health"
)

// NodeCandidate is a node with scheduling metadata.
type NodeCandidate struct {
	Config    config.NodeConfig
	Health    *health.NodeHealth
	Distance  float64
	LoadScore float64
}

// Scheduler selects the best edge node for a client request.
type Scheduler struct {
	nodes           []config.NodeConfig
	healthChecker   *health.Checker
	locator         geoip.Locator
	topN            int
	overloadThresh  float64
	rejectThresh    float64
}

// New creates a new scheduler.
func New(nodes []config.NodeConfig, hc *health.Checker, locator geoip.Locator, topN int, overloadThresh, rejectThresh float64) *Scheduler {
	return &Scheduler{
		nodes:          nodes,
		healthChecker:  hc,
		locator:        locator,
		topN:           topN,
		overloadThresh: overloadThresh,
		rejectThresh:   rejectThresh,
	}
}

// SelectNode picks the best node for the given client IP.
func (s *Scheduler) SelectNode(clientIP string) *config.NodeConfig {
	clientLoc := s.locator.Lookup(clientIP)

	// Build candidate list from healthy nodes
	var candidates []NodeCandidate
	for _, node := range s.nodes {
		nh := s.healthChecker.GetNode(node.Name)
		if nh == nil || nh.Status != health.StatusOnline {
			continue
		}

		loadScore := nh.LoadScore()
		if loadScore >= s.rejectThresh {
			continue // Skip overloaded nodes
		}

		dist := geoip.Distance(clientLoc.Lat, clientLoc.Lon, node.Lat, node.Lon)
		candidates = append(candidates, NodeCandidate{
			Config:    node,
			Health:    nh,
			Distance:  dist,
			LoadScore: loadScore,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Prefer same ISP
	ispCandidates := filterByISP(candidates, clientLoc.ISP)
	if len(ispCandidates) > 0 {
		candidates = ispCandidates
	}

	// Sort by distance
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Distance < candidates[j].Distance
	})

	// Take top N closest
	topN := s.topN
	if topN > len(candidates) {
		topN = len(candidates)
	}
	candidates = candidates[:topN]

	// Weighted selection by inverse load score among top N
	return weightedSelect(candidates, s.overloadThresh)
}

// SelectNodes returns multiple node candidates (for DNS round-robin responses).
func (s *Scheduler) SelectNodes(clientIP string, count int) []config.NodeConfig {
	clientLoc := s.locator.Lookup(clientIP)

	var candidates []NodeCandidate
	for _, node := range s.nodes {
		nh := s.healthChecker.GetNode(node.Name)
		if nh == nil || nh.Status != health.StatusOnline {
			continue
		}
		loadScore := nh.LoadScore()
		if loadScore >= s.rejectThresh {
			continue
		}
		dist := geoip.Distance(clientLoc.Lat, clientLoc.Lon, node.Lat, node.Lon)
		candidates = append(candidates, NodeCandidate{
			Config:    node,
			Health:    nh,
			Distance:  dist,
			LoadScore: loadScore,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	ispCandidates := filterByISP(candidates, clientLoc.ISP)
	if len(ispCandidates) > 0 {
		candidates = ispCandidates
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Distance < candidates[j].Distance
	})

	if count > len(candidates) {
		count = len(candidates)
	}

	result := make([]config.NodeConfig, count)
	for i := 0; i < count; i++ {
		result[i] = candidates[i].Config
	}
	return result
}

func filterByISP(candidates []NodeCandidate, isp string) []NodeCandidate {
	if isp == "" || isp == "unknown" {
		return nil
	}
	var filtered []NodeCandidate
	for _, c := range candidates {
		if c.Config.ISP == isp {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func weightedSelect(candidates []NodeCandidate, overloadThresh float64) *config.NodeConfig {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return &candidates[0].Config
	}

	// Weight = 1 - loadScore, reduce weight for overloaded nodes
	type weighted struct {
		node   *config.NodeConfig
		weight float64
	}
	var items []weighted
	totalWeight := 0.0
	for i := range candidates {
		w := 1.0 - candidates[i].LoadScore
		if candidates[i].LoadScore >= overloadThresh {
			w *= 0.1 // Significantly reduce weight
		}
		if w < 0.01 {
			w = 0.01
		}
		items = append(items, weighted{node: &candidates[i].Config, weight: w})
		totalWeight += w
	}

	r := rand.Float64() * totalWeight
	for _, item := range items {
		r -= item.weight
		if r <= 0 {
			return item.node
		}
	}
	return items[len(items)-1].node
}
