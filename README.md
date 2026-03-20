# cdn-scheduler

EdgeFlow CDN scheduler — DNS and HTTP-based request routing to optimal edge nodes.

## Features

- DNS server (miekg/dns) with intelligent scheduling
- HTTP 302 redirect scheduling (alternative)
- GeoIP-based geographic routing
- ISP-aware scheduling (telecom/unicom/mobile preference)
- Load-weighted node selection
- Active HTTP health checks with configurable thresholds
- Node status management (online/offline/maintenance)
- EDNS Client Subnet support

## Scheduling Algorithm

1. Filter healthy nodes (skip offline and overloaded)
2. Prefer same-ISP nodes
3. Sort by geographic distance to client
4. Select from top-N closest using load-weighted random

Load score: `CPU * 0.3 + Memory * 0.2 + Bandwidth * 0.4 + Connections * 0.1`

## Quick Start

```bash
# Build
go build -o bin/cdn-scheduler ./cmd

# Run
./bin/cdn-scheduler -config configs/scheduler-config.yaml

# Test DNS resolution
dig @localhost -p 15353 cdn.example.com.edgeflow.dev
```

## Testing

```bash
go test ./... -race -count=1
go test ./scheduler/... -bench=.
```
