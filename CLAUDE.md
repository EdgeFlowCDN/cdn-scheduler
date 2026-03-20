# cdn-scheduler

EdgeFlow CDN scheduler — DNS and HTTP request routing to optimal edge nodes.

## Tech Stack

Go 1.23, miekg/dns, MaxMind GeoIP (maxminddb-golang)

## Project Structure

```
cmd/          Entry point with graceful shutdown
config/       YAML config with node definitions
dns/          DNS + HTTP redirect servers
  server.go       DNS server (UDP+TCP), HTTP 302 redirect, EDNS Client Subnet
geoip/        Geographic IP lookup
  geoip.go        Static locator + MaxMind .mmdb locator with fallback
health/       Node health monitoring
  checker.go      Active HTTP health checks, failure/success thresholds,
                  maintenance mode, load score calculation
scheduler/    Core scheduling logic
  scheduler.go    ISP matching → distance sort → load-weighted selection
configs/      YAML config files
```

## Scheduling Algorithm

1. Filter healthy nodes (skip offline + overloaded at >0.95)
2. Prefer same-ISP nodes (telecom/unicom/mobile)
3. Sort by geographic distance
4. Pick from top-N closest using load-weighted random
5. Deprioritize nodes above 0.85 load score

Load score = CPU×0.3 + Mem×0.2 + Bandwidth×0.4 + Connections×0.1

## Running Tests

```bash
go test ./... -race -count=1
go test ./scheduler/... -bench=.
```
