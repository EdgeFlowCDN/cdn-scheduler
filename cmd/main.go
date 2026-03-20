package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/EdgeFlowCDN/cdn-scheduler/config"
	cdndns "github.com/EdgeFlowCDN/cdn-scheduler/dns"
	"github.com/EdgeFlowCDN/cdn-scheduler/geoip"
	"github.com/EdgeFlowCDN/cdn-scheduler/health"
	"github.com/EdgeFlowCDN/cdn-scheduler/scheduler"
)

func main() {
	configPath := flag.String("config", "configs/scheduler-config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize GeoIP locator
	var locator geoip.Locator
	if cfg.GeoIP.Database != "" {
		locator = geoip.NewMaxMindLocator(cfg.GeoIP.Database)
	} else {
		locator = geoip.NewStaticLocator()
	}

	// Initialize health checker
	hc := health.NewChecker(
		cfg.HealthCheck.Interval,
		cfg.HealthCheck.Timeout,
		cfg.HealthCheck.FailureThreshold,
		cfg.HealthCheck.SuccessThreshold,
	)
	for _, node := range cfg.Nodes {
		hc.RegisterNode(node.Name, node.IP, node.MaxBandwidth, node.MaxConnections, node.HealthEndpoint)
	}
	hc.Start()
	defer hc.Stop()

	// Initialize scheduler
	sched := scheduler.New(
		cfg.Nodes,
		hc,
		locator,
		cfg.DNS.TopN,
		cfg.LoadBalance.OverloadThreshold,
		cfg.LoadBalance.RejectThreshold,
	)

	// Start DNS server
	dnsServer := cdndns.NewServer(sched, cfg.DNS)
	go func() {
		if err := dnsServer.ListenAndServe(); err != nil {
			log.Fatalf("DNS server failed: %v", err)
		}
	}()

	// Start HTTP 302 redirect server
	httpServer := cdndns.NewHTTPRedirectServer(sched, cfg.HTTP.Listen)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatalf("HTTP redirect server failed: %v", err)
		}
	}()

	log.Printf("EdgeFlow Scheduler started (DNS: %s, HTTP: %s, nodes: %d)",
		cfg.DNS.Listen, cfg.HTTP.Listen, len(cfg.Nodes))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("shutting down")
}
