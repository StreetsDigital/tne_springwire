# Grafana Dashboards for TNE Catalyst Exchange

Production-ready Grafana dashboards for monitoring the Catalyst Ad Exchange.

## Quick Start

### 1. Start Monitoring Stack

```bash
cd grafana
docker-compose up -d
```

This starts:
- **Prometheus** at http://localhost:9090
- **Grafana** at http://localhost:3000

### 2. Start Your Exchange Server

```bash
# In another terminal
PUBLISHER_ALLOW_UNREGISTERED=true go run ./cmd/server
```

The server exposes metrics at http://localhost:8000/metrics

### 3. Access Grafana

Open http://localhost:3000 in your browser.

**Default credentials:**
- Username: `admin`
- Password: `admin`

**Pre-loaded dashboards:**
1. **Exchange Overview** - Request rates, latency, errors
2. **Circuit Breakers** - Bidder health and circuit breaker states
3. **System Health** - Go runtime metrics (memory, GC, goroutines)

## Dashboard Descriptions

### Exchange Overview

**Key metrics:**
- **Request Rate (QPS)** - Queries per second by endpoint
- **Auction QPS** - Auction-specific throughput
- **Error Rate** - Percentage of failed requests
- **Auction Latency** - P50/P95/P99 latency percentiles
- **HTTP Status Codes** - Status code distribution
- **Requests In-Flight** - Current concurrent requests
- **Active Connections** - HTTP connection count

**Thresholds:**
- ✅ P95 latency < 200ms (green)
- ⚠️ P95 latency 200-500ms (yellow)
- 🔴 P95 latency > 500ms (red)
- ✅ Error rate < 1% (green)
- 🔴 Error rate > 5% (red)

**Use cases:**
- Real-time performance monitoring
- Capacity planning
- SLA validation
- Incident detection

### Circuit Breakers

**Key metrics:**
- **Circuit Breaker States** - Visual state per bidder (CLOSED/OPEN/HALF-OPEN)
- **Bidder Request Rate** - Requests sent to each bidder
- **Circuit Breaker Results** - Success/failure/rejected counts
- **State Changes** - Circuit breaker transitions over time
- **Summary Table** - Complete circuit breaker statistics

**State meanings:**
- 🟢 **CLOSED (0)** - Healthy, requests passing through
- 🔴 **OPEN (1)** - Failing, requests rejected immediately
- 🟡 **HALF-OPEN (2)** - Testing recovery, limited requests

**Use cases:**
- Bidder health monitoring
- Cascade failure prevention
- Dependency health tracking
- Recovery validation

### System Health

**Key metrics:**
- **Memory Usage** - Heap allocated/in-use/idle
- **Goroutines** - Concurrent goroutine count
- **Heap Objects** - Number of allocated objects
- **Uptime** - Server uptime
- **GC Rate** - Garbage collection frequency
- **GC Pause Duration** - P95/P99 GC pause times
- **Allocation/Free Rate** - Memory churn
- **Goroutines & Threads** - Resource utilization

**Thresholds:**
- ✅ Goroutines < 50 (green)
- ⚠️ Goroutines 50-100 (yellow)
- 🔴 Goroutines > 100 (red)

**Use cases:**
- Memory leak detection
- Goroutine leak detection
- GC pressure monitoring
- Capacity planning

## Configuration

### Prometheus Scrape Config

Edit `prometheus.yml` to change scrape settings:

```yaml
global:
  scrape_interval: 15s  # How often to scrape metrics

scrape_configs:
  - job_name: 'catalyst-exchange'
    static_configs:
      - targets: ['host.docker.internal:8000']  # Your server address
```

### Grafana Settings

Edit `docker-compose.yml` to customize Grafana:

```yaml
environment:
  - GF_SECURITY_ADMIN_PASSWORD=your-password  # Change default password
  - GF_USERS_ALLOW_SIGN_UP=false              # Disable user signup
  - GF_AUTH_ANONYMOUS_ENABLED=true            # Allow anonymous viewing
```

## Custom Dashboards

### Importing Additional Dashboards

1. Go to http://localhost:3000/dashboard/import
2. Upload a dashboard JSON file or paste JSON
3. Select "Prometheus" as the data source

### Exporting Dashboards

1. Open a dashboard
2. Click "Share" → "Export"
3. Save JSON to `grafana/dashboards/`

### Auto-Provisioning

Dashboards in `grafana/dashboards/` are automatically loaded at startup.

To add a new dashboard:
1. Place JSON file in `grafana/dashboards/`
2. Restart Grafana: `docker-compose restart grafana`

## Alerting (Future)

Grafana supports alerts based on metric thresholds. Example alert conditions:

- **High Error Rate**: `pbs_http_requests_total{code!="200"} / pbs_http_requests_total > 0.05`
- **High Latency**: `histogram_quantile(0.95, pbs_http_request_duration_seconds_bucket) > 0.200`
- **Circuit Breaker Open**: `pbs_bidder_circuit_breaker_state == 1`
- **Memory Growth**: `increase(go_memstats_heap_alloc_bytes[5m]) > 100MB`

Configure alerts in Grafana UI or via provisioning files.

## Troubleshooting

### Prometheus not scraping metrics

**Check Prometheus targets:**
```bash
open http://localhost:9090/targets
```

Should show `catalyst-exchange` as UP.

**If DOWN:**
- Ensure your server is running on port 8000
- Check server exposes `/metrics` endpoint: `curl http://localhost:8000/metrics`
- Verify Docker can reach host: `docker exec catalyst-prometheus ping host.docker.internal`

### Dashboards showing "No data"

**Verify Prometheus has data:**
```bash
open http://localhost:9090/graph
```

Run query: `up{job="catalyst-exchange"}`

**If no results:**
- Restart Prometheus: `docker-compose restart prometheus`
- Check Prometheus logs: `docker-compose logs prometheus`

### Grafana won't start

**Check logs:**
```bash
docker-compose logs grafana
```

**Common issues:**
- Port 3000 already in use: Change port in docker-compose.yml
- Permissions on volumes: `sudo chown -R 472:472 grafana-data/`

## Production Deployment

### Security Recommendations

1. **Change default password:**
   ```yaml
   - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_PASSWORD}
   ```

2. **Disable anonymous access:**
   ```yaml
   - GF_AUTH_ANONYMOUS_ENABLED=false
   ```

3. **Enable HTTPS:**
   ```yaml
   - GF_SERVER_PROTOCOL=https
   - GF_SERVER_CERT_FILE=/etc/ssl/grafana.crt
   - GF_SERVER_CERT_KEY=/etc/ssl/grafana.key
   ```

4. **Use external Prometheus:**
   Update datasource URL in `provisioning/datasources/prometheus.yml`

### High Availability

For production HA setup:
- Use external Prometheus (with remote storage)
- Use external Grafana database (PostgreSQL/MySQL)
- Load balance multiple Grafana instances
- Set up Prometheus federation for multi-region

## Metrics Reference

### PBS Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `pbs_http_requests_total` | Counter | Total HTTP requests by method/path/code |
| `pbs_http_request_duration_seconds` | Histogram | Request latency distribution |
| `pbs_http_requests_in_flight` | Gauge | Current in-flight requests |
| `pbs_active_connections` | Gauge | Active HTTP connections |
| `pbs_bidder_circuit_breaker_state` | Gauge | Circuit breaker state (0/1/2) |
| `pbs_bidder_circuit_breaker_requests_total` | Counter | Total requests through circuit breaker |
| `pbs_bidder_circuit_breaker_successes_total` | Counter | Successful requests |
| `pbs_bidder_circuit_breaker_failures_total` | Counter | Failed requests |
| `pbs_bidder_circuit_breaker_rejected_total` | Counter | Rejected requests (circuit open) |
| `pbs_bidder_circuit_breaker_state_changes_total` | Counter | Circuit state transitions |
| `pbs_auth_failures_total` | Counter | Authentication failures |
| `pbs_rate_limit_rejected_total` | Counter | Rate-limited requests |

### Go Runtime Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `go_goroutines` | Gauge | Number of goroutines |
| `go_memstats_heap_alloc_bytes` | Gauge | Bytes allocated on heap |
| `go_memstats_heap_inuse_bytes` | Gauge | Heap bytes in use |
| `go_memstats_heap_objects` | Gauge | Number of allocated objects |
| `go_gc_duration_seconds` | Summary | GC pause duration |
| `process_start_time_seconds` | Gauge | Process start time |

## Support

For issues or questions:
- Check logs: `docker-compose logs -f`
- Verify metrics: http://localhost:8000/metrics
- Check Prometheus: http://localhost:9090
- Grafana docs: https://grafana.com/docs/

## Cleanup

Stop and remove all containers:
```bash
docker-compose down
```

Remove volumes (deletes all data):
```bash
docker-compose down -v
```
