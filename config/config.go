package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DNS         DNSConfig         `yaml:"dns"`
	HTTP        HTTPConfig        `yaml:"http"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	GeoIP       GeoIPConfig       `yaml:"geoip"`
	LoadBalance LoadBalanceConfig `yaml:"load_balance"`
	Nodes       []NodeConfig      `yaml:"nodes"`
}

type DNSConfig struct {
	Listen string `yaml:"listen"`
	TTL    uint32 `yaml:"ttl"`
	Domain string `yaml:"domain"` // e.g., "edgeflow.dev"
	TopN   int    `yaml:"top_n"`
}

type HTTPConfig struct {
	Listen string `yaml:"listen"`
}

type HealthCheckConfig struct {
	Interval         time.Duration `yaml:"interval"`
	Timeout          time.Duration `yaml:"timeout"`
	FailureThreshold int           `yaml:"failure_threshold"`
	SuccessThreshold int           `yaml:"success_threshold"`
}

type GeoIPConfig struct {
	Database string `yaml:"database"`
}

type LoadBalanceConfig struct {
	OverloadThreshold float64 `yaml:"overload_threshold"`
	RejectThreshold   float64 `yaml:"reject_threshold"`
}

type NodeConfig struct {
	Name           string  `yaml:"name"`
	IP             string  `yaml:"ip"`
	Region         string  `yaml:"region"`
	ISP            string  `yaml:"isp"`
	Lat            float64 `yaml:"lat"`
	Lon            float64 `yaml:"lon"`
	MaxBandwidth   int64   `yaml:"max_bandwidth"`
	MaxConnections int64   `yaml:"max_connections"`
	HealthEndpoint string  `yaml:"health_endpoint"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Defaults
	if cfg.DNS.Listen == "" {
		cfg.DNS.Listen = ":53"
	}
	if cfg.DNS.TTL == 0 {
		cfg.DNS.TTL = 60
	}
	if cfg.DNS.TopN == 0 {
		cfg.DNS.TopN = 3
	}
	if cfg.HTTP.Listen == "" {
		cfg.HTTP.Listen = ":8053"
	}
	if cfg.HealthCheck.Interval == 0 {
		cfg.HealthCheck.Interval = 10 * time.Second
	}
	if cfg.HealthCheck.Timeout == 0 {
		cfg.HealthCheck.Timeout = 3 * time.Second
	}
	if cfg.HealthCheck.FailureThreshold == 0 {
		cfg.HealthCheck.FailureThreshold = 3
	}
	if cfg.HealthCheck.SuccessThreshold == 0 {
		cfg.HealthCheck.SuccessThreshold = 2
	}
	if cfg.LoadBalance.OverloadThreshold == 0 {
		cfg.LoadBalance.OverloadThreshold = 0.85
	}
	if cfg.LoadBalance.RejectThreshold == 0 {
		cfg.LoadBalance.RejectThreshold = 0.95
	}

	if len(cfg.Nodes) == 0 {
		return nil, fmt.Errorf("at least one node must be configured")
	}

	return &cfg, nil
}
