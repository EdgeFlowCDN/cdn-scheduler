package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	// Close GeoIP database if it supports closing
	if cl, ok := locator.(interface{ Close() }); ok {
		defer cl.Close()
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
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP redirect server failed: %v", err)
		}
	}()

	log.Printf("EdgeFlow Scheduler started (DNS: %s, HTTP: %s, nodes: %d)",
		cfg.DNS.Listen, cfg.HTTP.Listen, len(cfg.Nodes))

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("received signal %v, shutting down gracefully...", sig)

	// Graceful shutdown: stop health checker
	hc.Stop()
	log.Println("health checker stopped")

	// Shutdown HTTP redirect server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP redirect server forced to shutdown: %v", err)
	} else {
		log.Println("HTTP redirect server stopped")
	}

	// Shutdown DNS server
	dnsServer.Shutdown()
	log.Println("DNS server stopped")

	log.Println("EdgeFlow Scheduler shut down cleanly")
}
