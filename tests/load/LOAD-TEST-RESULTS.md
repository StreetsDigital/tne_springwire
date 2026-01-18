# Load Test Results

This document tracks load test results over time to monitor performance trends and regressions.

## Test Environment

| Component | Specification |
|-----------|--------------|
| **CPU** | Intel(R) Xeon(R) CPU E5-2697 v2 @ 2.70GHz (24 cores) |
| **Memory** | TBD |
| **OS** | Darwin (macOS) |
| **Go Version** | 1.25.5 |
| **Database** | PostgreSQL 15.x |
| **Cache** | Redis 7.x |

## Performance Targets

| Metric | Target | Critical Threshold |
|--------|--------|-------------------|
| **Success Rate** | > 99% | > 95% |
| **P50 Latency** | < 50ms | < 100ms |
| **P95 Latency** | < 150ms | < 300ms |
| **P99 Latency** | < 300ms | < 1000ms |
| **Max Throughput** | > 10k QPS | > 5k QPS |
| **Error Rate (spike)** | < 5% | < 10% |

## Test Results

### Baseline Test (2026-01-17)

**Configuration:**
- Duration: 5 minutes
- Pattern: Ramp to 100 VUs, sustain, ramp down
- Target QPS: 1k-10k

**Results:**
```
Status: Not yet run
Run with: ./tests/load/run-load-tests.sh baseline
```

### Spike Test (2026-01-17)

**Configuration:**
- Duration: 2 minutes
- Pattern: 50 VUs → 1000 VUs (20x spike) → 50 VUs
- Peak QPS: ~50k

**Results:**
```
Status: Not yet run
Run with: ./tests/load/run-load-tests.sh spike
```

### Soak Test (2026-01-17)

**Configuration:**
- Duration: 1 hour (extendable to 24h)
- Sustained load: 5k QPS
- VUs: 200

**Results:**
```
Status: Not yet run
Run with: ./tests/load/run-load-tests.sh soak
```

### Stress Test (2026-01-17)

**Configuration:**
- Duration: 10 minutes
- Pattern: Ramp from 100 → 10,000 VUs
- Purpose: Find breaking point

**Results:**
```
Status: Not yet run
Run with: ./tests/load/run-load-tests.sh stress
```

---

## How to Add Results

After running a load test:

1. **Copy the test output** to this file under the appropriate section
2. **Include key metrics:**
   - Total requests
   - Success rate
   - P50/P95/P99 latencies
   - Actual QPS achieved
   - Error breakdown
3. **Note any issues:**
   - Circuit breaker activations
   - Memory growth
   - Database connection issues
   - Timeouts
4. **Compare against targets** and document pass/fail

### Result Template

```markdown
### Test Name (YYYY-MM-DD HH:MM)

**Configuration:**
- Duration: X minutes
- VUs: Y
- Target QPS: Z

**Results:**
Total Requests:  XXX,XXX
Success Rate:    XX.XX%
Error Rate:      X.XX%

Latency:
- P50:  XX ms
- P95:  XX ms
- P99:  XXX ms
- Max:  XXX ms

Actual QPS: XX,XXX

**Issues:**
- Circuit breaker "bidder_X" opened at T+5m
- Memory growth: 500MB → 750MB over 1 hour
- Database connection pool exhausted at peak load

**Pass/Fail:** ✅ PASS / ❌ FAIL
**Notes:** Any additional observations
```

---

## Trends and Analysis

### Performance Over Time

_Track how metrics change across test runs_

| Date | Test | QPS | P95 | P99 | Success Rate | Notes |
|------|------|-----|-----|-----|--------------|-------|
| 2026-01-17 | Baseline | - | - | - | - | Initial baseline |

### Regressions

_Document any performance degradations_

| Date | Issue | Impact | Root Cause | Resolution |
|------|-------|--------|------------|------------|
| - | - | - | - | - |

### Improvements

_Track performance improvements_

| Date | Change | Improvement | Metrics |
|------|--------|-------------|---------|
| - | - | - | - |

---

## Load Test Checklist

Before running load tests:

- [ ] Server is running (`make run`)
- [ ] Database is accessible
- [ ] Redis is accessible
- [ ] Metrics endpoint is working (`/metrics`)
- [ ] Health check passes (`/health`)
- [ ] Disk space available for results
- [ ] Monitoring dashboards are ready (if available)

After running load tests:

- [ ] Results saved to `results/` directory
- [ ] Key metrics documented in this file
- [ ] Prometheus metrics reviewed
- [ ] Circuit breaker stats checked
- [ ] Memory usage analyzed
- [ ] Errors investigated
- [ ] Results compared against targets
- [ ] Performance trends updated

---

## Next Steps

After establishing baselines:

1. **Set up alerts** based on observed thresholds
2. **Create Grafana dashboards** for real-time monitoring
3. **Run tests regularly** (weekly/before releases)
4. **Tune performance** based on bottlenecks found
5. **Document** capacity planning recommendations

---

## References

- [Load Test README](./README.md) - How to run tests
- [Performance Benchmarks](../../PERFORMANCE-BENCHMARKS.md) - Unit-level benchmarks
- k6 Documentation: https://k6.io/docs/
