# Load Testing Guide

This directory contains load tests for the TNE Catalyst ad exchange.

## Prerequisites

### Option 1: k6 (Recommended)
```bash
# macOS
brew install k6

# Linux
sudo apt-get install k6

# Or download from: https://k6.io/docs/getting-started/installation/
```

### Option 2: Go-based Load Test
```bash
# No additional dependencies - uses Go's built-in testing
go test -v ./tests/load -tags=loadtest
```

## Test Scenarios

| Test | Description | Duration | Target QPS | Purpose |
|------|-------------|----------|------------|---------|
| **baseline.js** | Steady load test | 5 min | 1k-10k | Establish baseline performance |
| **spike.js** | Sudden traffic spike | 2 min | 50k burst | Test auto-scaling and resilience |
| **soak.js** | Extended stability | 1-24 hours | 5k sustained | Find memory leaks and degradation |
| **stress.js** | Find breaking point | 10 min | Ramp to failure | Determine max capacity |
| **load_test.go** | Go-based alternative | Configurable | Configurable | Native Go load testing |

## Quick Start

### 1. Start the Server
```bash
# Terminal 1: Start the ad exchange
cd /Users/andrewstreets/tne-catalyst
make run

# Or with Docker
docker-compose up
```

### 2. Run a Load Test

**Baseline Test (1k QPS for 5 minutes):**
```bash
k6 run tests/load/baseline.js
```

**Spike Test (50k QPS burst):**
```bash
k6 run tests/load/spike.js
```

**Soak Test (5k QPS for 1 hour):**
```bash
k6 run --duration 1h tests/load/soak.js
```

**Stress Test (find limits):**
```bash
k6 run tests/load/stress.js
```

**Go-based Load Test:**
```bash
go test -v ./tests/load -tags=loadtest -timeout 30m \
  -qps=1000 \
  -duration=5m \
  -endpoint=http://localhost:8080/openrtb2/auction
```

## Test Configuration

Edit the test files to customize:
- Target URL (`BASE_URL` environment variable)
- Virtual users (VUs)
- Ramp-up/ramp-down patterns
- Request rate (RPS/QPS)
- Test duration

Example:
```bash
BASE_URL=http://production.example.com:8080 k6 run tests/load/baseline.js
```

## Interpreting Results

### k6 Metrics

Key metrics to watch:

```
http_req_duration..........: avg=45ms  min=12ms med=38ms max=1.2s p(90)=78ms p(95)=112ms
http_req_failed............: 0.12%    ✓ 145   ✗ 119,855
http_reqs..................: 120000   2000/s
vus........................: 100      min=10  max=200
```

**Good:**
- `http_req_failed` < 1%
- `p(95)` latency < 200ms
- `p(99)` latency < 500ms
- No increasing trend in latency over time

**Bad:**
- `http_req_failed` > 5%
- `p(95)` latency > 500ms
- Latency increasing over time (memory leak)
- Error rate increasing

### Performance Targets

| Metric | Target | Critical |
|--------|--------|----------|
| **Success Rate** | > 99% | > 95% |
| **P50 Latency** | < 50ms | < 100ms |
| **P95 Latency** | < 150ms | < 300ms |
| **P99 Latency** | < 300ms | < 1000ms |
| **Throughput** | 10k QPS | 5k QPS |

## Monitoring During Tests

### Prometheus Metrics
```bash
# View real-time metrics
open http://localhost:8080/metrics

# Query specific metrics
curl http://localhost:8080/metrics | grep auction_
```

### System Resources
```bash
# Monitor CPU/Memory
docker stats

# Or native
top -pid $(pgrep -f catalyst)
```

### Circuit Breakers
```bash
# Check circuit breaker status
curl http://localhost:8080/admin/circuit-breakers | jq
```

## Common Issues

### 1. "Connection refused"
**Problem:** Server not running
**Solution:**
```bash
make run
# Or check: lsof -i :8080
```

### 2. High error rate (> 5%)
**Problem:** Server overloaded or misconfigured
**Solution:**
- Check server logs: `docker logs catalyst`
- Reduce load: Edit test file, reduce VUs or rate
- Check database connections
- Verify circuit breakers aren't all open

### 3. Increasing latency over time
**Problem:** Memory leak or resource exhaustion
**Solution:**
- Profile the application: `curl http://localhost:8080/debug/pprof/heap > heap.prof`
- Check for goroutine leaks: `curl http://localhost:8080/debug/pprof/goroutine > goroutine.prof`
- Review memory growth in Prometheus

### 4. "Socket: too many open files"
**Problem:** File descriptor limit
**Solution:**
```bash
# macOS
ulimit -n 10000

# Linux
sudo sysctl -w fs.file-max=100000
```

## Advanced Usage

### Custom Scenarios

Create custom test scenarios by modifying the existing files or creating new ones:

```javascript
import http from 'k6/http';
import { check } from 'k6';

export let options = {
  stages: [
    { duration: '30s', target: 100 },  // Ramp to 100 VUs
    { duration: '5m', target: 100 },   // Stay at 100 VUs
    { duration: '30s', target: 0 },    // Ramp down
  ],
};

export default function () {
  const payload = JSON.stringify({
    id: `req-${__VU}-${__ITER}`,
    site: { domain: 'example.com' },
    imp: [{ id: '1', banner: { w: 300, h: 250 } }],
  });

  const res = http.post('http://localhost:8080/openrtb2/auction', payload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
    'has bids': (r) => JSON.parse(r.body).seatbid.length > 0,
  });
}
```

### Distributed Load Testing

For very high load (>100k QPS), use k6 cloud or distributed mode:

```bash
# Master node
k6 run --execution-mode=distributed --master tests/load/baseline.js

# Worker nodes (on different machines)
k6 run --execution-mode=distributed --worker --master-host=MASTER_IP tests/load/baseline.js
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Load Test
on:
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM
  workflow_dispatch:

jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: grafana/setup-k6-action@v1
      - name: Run load test
        run: k6 run tests/load/baseline.js
      - name: Upload results
        uses: actions/upload-artifact@v2
        with:
          name: load-test-results
          path: summary.json
```

## Results Storage

k6 can export results to various backends:

```bash
# JSON file
k6 run --out json=results.json tests/load/baseline.js

# InfluxDB
k6 run --out influxdb=http://localhost:8086/k6 tests/load/baseline.js

# Prometheus
k6 run --out experimental-prometheus-rw=http://localhost:9090/api/v1/write tests/load/baseline.js

# Cloud
k6 cloud tests/load/baseline.js
```

## Troubleshooting

### Enable Debug Logging
```bash
k6 run --http-debug tests/load/baseline.js
```

### Generate HTTP Archive (HAR)
```bash
k6 run --http-debug=full tests/load/baseline.js 2>&1 | grep -A 100 "Request:"
```

### Profile k6 Itself
```bash
k6 run --compatibility-mode=base tests/load/baseline.js
```

## Next Steps

After running load tests:

1. **Analyze results** - Compare against performance benchmarks
2. **Identify bottlenecks** - Use profiling and metrics
3. **Optimize** - Based on findings (database, caching, etc.)
4. **Re-test** - Verify improvements
5. **Document** - Update capacity planning docs
6. **Set alerts** - Configure PagerDuty thresholds

## References

- [k6 Documentation](https://k6.io/docs/)
- [k6 Examples](https://k6.io/docs/examples/)
- [Grafana k6 Cloud](https://k6.io/cloud/)
