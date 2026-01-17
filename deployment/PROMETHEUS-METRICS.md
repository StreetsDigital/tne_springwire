# Prometheus Metrics Guide

## Overview

This guide documents all Prometheus metrics exported by TNE Catalyst for monitoring production performance, auction health, and circuit breaker states.

**Metrics Endpoint**: `http://localhost:8000/metrics`

---

## Table of Contents

1. [HTTP Request Metrics](#http-request-metrics)
2. [Auction Metrics](#auction-metrics)
3. [Bidder Metrics](#bidder-metrics)
4. [Circuit Breaker Metrics](#circuit-breaker-metrics) ⭐ NEW
5. [IDR (Intelligent Demand Router) Metrics](#idr-metrics)
6. [Privacy & Consent Metrics](#privacy--consent-metrics)
7. [System Metrics](#system-metrics)
8. [Revenue & Margin Metrics](#revenue--margin-metrics)
9. [Grafana Dashboard Examples](#grafana-dashboard-examples)
10. [Alert Rules](#alert-rules)

---

## HTTP Request Metrics

### `pbs_http_requests_total`
**Type**: Counter
**Labels**: `method`, `path`, `status`
**Description**: Total number of HTTP requests received

**Example**:
```promql
# Total requests by endpoint
sum by (path) (pbs_http_requests_total)

# 4xx error rate
rate(pbs_http_requests_total{status=~"4.."}[5m])

# 5xx error rate
rate(pbs_http_requests_total{status=~"5.."}[5m])
```

### `pbs_http_request_duration_seconds`
**Type**: Histogram
**Labels**: `method`, `path`
**Description**: HTTP request latency in seconds

**Example**:
```promql
# P95 latency by endpoint
histogram_quantile(0.95, sum by (path, le) (rate(pbs_http_request_duration_seconds_bucket[5m])))

# Average response time
rate(pbs_http_request_duration_seconds_sum[5m]) / rate(pbs_http_request_duration_seconds_count[5m])
```

### `pbs_http_requests_in_flight`
**Type**: Gauge
**Description**: Number of HTTP requests currently being processed

**Example**:
```promql
# Current in-flight requests
pbs_http_requests_in_flight

# Max in-flight requests (last hour)
max_over_time(pbs_http_requests_in_flight[1h])
```

---

## Auction Metrics

### `pbs_auctions_total`
**Type**: Counter
**Labels**: `status`, `media_type`
**Description**: Total number of auctions processed

**Example**:
```promql
# Auction success rate
rate(pbs_auctions_total{status="success"}[5m]) / rate(pbs_auctions_total[5m])

# Auctions by media type
sum by (media_type) (rate(pbs_auctions_total[5m]))
```

### `pbs_auction_duration_seconds`
**Type**: Histogram
**Labels**: `media_type`
**Description**: Auction processing duration in seconds

**Example**:
```promql
# P99 auction latency
histogram_quantile(0.99, sum by (media_type, le) (rate(pbs_auction_duration_seconds_bucket[5m])))

# Average auction time by media type
rate(pbs_auction_duration_seconds_sum[5m]) / rate(pbs_auction_duration_seconds_count[5m])
```

### `pbs_bids_received_total`
**Type**: Counter
**Labels**: `bidder`, `media_type`
**Description**: Total number of bids received from bidders

**Example**:
```promql
# Bids per second by bidder
rate(pbs_bids_received_total[5m])

# Top bidders by volume
topk(10, sum by (bidder) (rate(pbs_bids_received_total[5m])))
```

### `pbs_bid_cpm`
**Type**: Histogram
**Labels**: `bidder`, `media_type`
**Description**: Bid CPM (cost per mille) distribution

**Example**:
```promql
# Average CPM by bidder
rate(pbs_bid_cpm_sum[5m]) / rate(pbs_bid_cpm_count[5m])

# P90 CPM
histogram_quantile(0.90, sum by (bidder, le) (rate(pbs_bid_cpm_bucket[5m])))
```

### `pbs_bidders_selected`
**Type**: Histogram
**Labels**: `media_type`
**Description**: Number of bidders selected per auction

**Example**:
```promql
# Average bidders per auction
rate(pbs_bidders_selected_sum[5m]) / rate(pbs_bidders_selected_count[5m])
```

### `pbs_bidders_excluded`
**Type**: Histogram
**Labels**: `reason`
**Description**: Number of bidders excluded per auction

**Example**:
```promql
# Exclusions by reason
sum by (reason) (rate(pbs_bidders_excluded_sum[5m]))
```

---

## Bidder Metrics

### `pbs_bidder_requests_total`
**Type**: Counter
**Labels**: `bidder`
**Description**: Total requests sent to each bidder

**Example**:
```promql
# Requests per second by bidder
rate(pbs_bidder_requests_total[5m])

# Total requests (last hour)
increase(pbs_bidder_requests_total[1h])
```

### `pbs_bidder_latency_seconds`
**Type**: Histogram
**Labels**: `bidder`
**Description**: Bidder response latency in seconds

**Example**:
```promql
# P95 latency by bidder
histogram_quantile(0.95, sum by (bidder, le) (rate(pbs_bidder_latency_seconds_bucket[5m])))

# Slow bidders (>150ms P95)
histogram_quantile(0.95, sum by (bidder, le) (rate(pbs_bidder_latency_seconds_bucket[5m]))) > 0.150
```

### `pbs_bidder_errors_total`
**Type**: Counter
**Labels**: `bidder`, `error_type`
**Description**: Total errors from bidders

**Example**:
```promql
# Error rate by bidder
rate(pbs_bidder_errors_total[5m])

# Errors by type
sum by (error_type) (rate(pbs_bidder_errors_total[5m]))
```

### `pbs_bidder_timeouts_total`
**Type**: Counter
**Labels**: `bidder`
**Description**: Total timeout events from bidders

**Example**:
```promql
# Timeout rate
rate(pbs_bidder_timeouts_total[5m])

# Bidders with high timeout rate (>5%)
rate(pbs_bidder_timeouts_total[5m]) / rate(pbs_bidder_requests_total[5m]) > 0.05
```

---

## Circuit Breaker Metrics ⭐ NEW

### `pbs_bidder_circuit_breaker_state`
**Type**: Gauge
**Labels**: `bidder`
**Description**: Current circuit breaker state per bidder (0=closed, 1=open, 2=half-open)

**Example**:
```promql
# Count bidders with open circuits
count(pbs_bidder_circuit_breaker_state == 1)

# List bidders in open state
pbs_bidder_circuit_breaker_state == 1

# Bidders in half-open state (testing recovery)
pbs_bidder_circuit_breaker_state == 2
```

### `pbs_bidder_circuit_breaker_requests_total`
**Type**: Counter
**Labels**: `bidder`
**Description**: Total requests processed through circuit breaker

**Example**:
```promql
# Request rate through circuit breaker
rate(pbs_bidder_circuit_breaker_requests_total[5m])
```

### `pbs_bidder_circuit_breaker_failures_total`
**Type**: Counter
**Labels**: `bidder`
**Description**: Total failures recorded by circuit breaker

**Example**:
```promql
# Failure rate by bidder
rate(pbs_bidder_circuit_breaker_failures_total[5m])

# Bidders with high failure rate (>10%)
rate(pbs_bidder_circuit_breaker_failures_total[5m]) / rate(pbs_bidder_circuit_breaker_requests_total[5m]) > 0.10
```

### `pbs_bidder_circuit_breaker_successes_total`
**Type**: Counter
**Labels**: `bidder`
**Description**: Total successes recorded by circuit breaker

**Example**:
```promql
# Success rate by bidder
rate(pbs_bidder_circuit_breaker_successes_total[5m]) / rate(pbs_bidder_circuit_breaker_requests_total[5m])
```

### `pbs_bidder_circuit_breaker_rejected_total`
**Type**: Counter
**Labels**: `bidder`
**Description**: Total requests rejected because circuit was open

**Example**:
```promql
# Rejection rate (indicates circuit breaker activations)
rate(pbs_bidder_circuit_breaker_rejected_total[5m])

# Total rejected requests (last hour)
increase(pbs_bidder_circuit_breaker_rejected_total[1h])
```

### `pbs_bidder_circuit_breaker_state_changes_total`
**Type**: Counter
**Labels**: `bidder`, `from_state`, `to_state`
**Description**: Total circuit breaker state transitions

**Example**:
```promql
# Circuit opening events (closed -> open)
rate(pbs_bidder_circuit_breaker_state_changes_total{from_state="closed", to_state="open"}[5m])

# Circuit recovery events (half-open -> closed)
rate(pbs_bidder_circuit_breaker_state_changes_total{from_state="half-open", to_state="closed"}[5m])

# Frequent state changes (flapping detection)
sum by (bidder) (rate(pbs_bidder_circuit_breaker_state_changes_total[5m])) > 0.1
```

**Circuit Breaker Health Dashboard**:
```promql
# Overall circuit breaker health score
(
  count(pbs_bidder_circuit_breaker_state == 0) /  # Closed circuits
  count(pbs_bidder_circuit_breaker_state)         # Total circuits
) * 100
```

---

## IDR Metrics

### `pbs_idr_requests_total`
**Type**: Counter
**Labels**: `status`
**Description**: Total requests to IDR service

**Example**:
```promql
# IDR request rate
rate(pbs_idr_requests_total[5m])

# IDR success rate
rate(pbs_idr_requests_total{status="success"}[5m]) / rate(pbs_idr_requests_total[5m])
```

### `pbs_idr_latency_seconds`
**Type**: Histogram
**Description**: IDR service latency in seconds

**Example**:
```promql
# P99 IDR latency
histogram_quantile(0.99, rate(pbs_idr_latency_seconds_bucket[5m]))
```

### `pbs_idr_circuit_breaker_state`
**Type**: Gauge
**Description**: IDR circuit breaker state (0=closed, 1=open, 2=half-open)

**Example**:
```promql
# IDR circuit status
pbs_idr_circuit_breaker_state
```

---

## Privacy & Consent Metrics

### `pbs_privacy_filtered_total`
**Type**: Counter
**Labels**: `bidder`, `reason`
**Description**: Total bidders filtered due to privacy regulations

**Example**:
```promql
# Privacy filtering rate
rate(pbs_privacy_filtered_total[5m])

# Filters by regulation
sum by (reason) (rate(pbs_privacy_filtered_total[5m]))
```

### `pbs_consent_signals_total`
**Type**: Counter
**Labels**: `type`, `has_consent`
**Description**: Consent signals received (GDPR, CCPA, etc.)

**Example**:
```promql
# Consent rate by type
rate(pbs_consent_signals_total{has_consent="yes"}[5m]) / rate(pbs_consent_signals_total[5m])
```

---

## System Metrics

### `pbs_active_connections`
**Type**: Gauge
**Description**: Number of active connections

**Example**:
```promql
# Current connections
pbs_active_connections

# Peak connections (last hour)
max_over_time(pbs_active_connections[1h])
```

### `pbs_rate_limit_rejected_total`
**Type**: Counter
**Description**: Total requests rejected due to rate limiting

**Example**:
```promql
# Rate limit rejection rate
rate(pbs_rate_limit_rejected_total[5m])
```

### `pbs_auth_failures_total`
**Type**: Counter
**Description**: Total authentication failures

**Example**:
```promql
# Auth failure rate
rate(pbs_auth_failures_total[5m])
```

---

## Revenue & Margin Metrics

### `pbs_revenue_total`
**Type**: Counter
**Labels**: `publisher`, `bidder`, `media_type`
**Description**: Total bid revenue (before multiplier adjustment)

**Example**:
```promql
# Revenue per second
rate(pbs_revenue_total[5m])

# Revenue by publisher
sum by (publisher) (rate(pbs_revenue_total[5m]))
```

### `pbs_publisher_payout_total`
**Type**: Counter
**Labels**: `publisher`, `bidder`, `media_type`
**Description**: Total payout to publishers (after multiplier)

**Example**:
```promql
# Publisher payout rate
rate(pbs_publisher_payout_total[5m])
```

### `pbs_platform_margin_total`
**Type**: Counter
**Labels**: `publisher`, `bidder`, `media_type`
**Description**: Total platform margin/revenue

**Example**:
```promql
# Platform revenue per second
rate(pbs_platform_margin_total[5m])

# Top revenue-generating bidders
topk(10, sum by (bidder) (rate(pbs_platform_margin_total[5m])))
```

### `pbs_margin_percentage`
**Type**: Histogram
**Labels**: `publisher`
**Description**: Platform margin percentage distribution

**Example**:
```promql
# Average margin by publisher
rate(pbs_margin_percentage_sum[5m]) / rate(pbs_margin_percentage_count[5m])
```

### `pbs_floor_adjustments_total`
**Type**: Counter
**Labels**: `publisher`
**Description**: Number of floor price adjustments applied

**Example**:
```promql
# Floor adjustments per second
rate(pbs_floor_adjustments_total[5m])
```

---

## Grafana Dashboard Examples

### Circuit Breaker Health Dashboard

```json
{
  "title": "Circuit Breaker Health",
  "panels": [
    {
      "title": "Circuit Breaker States",
      "targets": [
        {
          "expr": "count by (bidder) (pbs_bidder_circuit_breaker_state == 0)",
          "legendFormat": "Closed"
        },
        {
          "expr": "count by (bidder) (pbs_bidder_circuit_breaker_state == 1)",
          "legendFormat": "Open"
        },
        {
          "expr": "count by (bidder) (pbs_bidder_circuit_breaker_state == 2)",
          "legendFormat": "Half-Open"
        }
      ]
    },
    {
      "title": "Failure Rate by Bidder",
      "targets": [
        {
          "expr": "rate(pbs_bidder_circuit_breaker_failures_total[5m]) / rate(pbs_bidder_circuit_breaker_requests_total[5m])",
          "legendFormat": "{{bidder}}"
        }
      ]
    },
    {
      "title": "Rejected Requests (Circuit Open)",
      "targets": [
        {
          "expr": "rate(pbs_bidder_circuit_breaker_rejected_total[5m])",
          "legendFormat": "{{bidder}}"
        }
      ]
    }
  ]
}
```

### Auction Performance Dashboard

```json
{
  "title": "Auction Performance",
  "panels": [
    {
      "title": "Auction Rate",
      "targets": [
        {
          "expr": "rate(pbs_auctions_total[5m])",
          "legendFormat": "{{status}}"
        }
      ]
    },
    {
      "title": "P95 Auction Latency",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum by (media_type, le) (rate(pbs_auction_duration_seconds_bucket[5m])))",
          "legendFormat": "{{media_type}}"
        }
      ]
    },
    {
      "title": "Bidder Performance",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum by (bidder, le) (rate(pbs_bidder_latency_seconds_bucket[5m])))",
          "legendFormat": "{{bidder}}"
        }
      ]
    }
  ]
}
```

---

## Alert Rules

### Critical Alerts

```yaml
groups:
  - name: circuit_breakers
    rules:
      - alert: MultipleCircuitBreakersOpen
        expr: count(pbs_bidder_circuit_breaker_state == 1) >= 3
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Multiple circuit breakers are open"
          description: "{{ $value }} bidder circuit breakers are in open state"

      - alert: CircuitBreakerFlapping
        expr: rate(pbs_bidder_circuit_breaker_state_changes_total[5m]) > 0.1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Circuit breaker frequently changing state"
          description: "Circuit breaker for {{ $labels.bidder }} is flapping"

      - alert: HighRejectionRate
        expr: rate(pbs_bidder_circuit_breaker_rejected_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High circuit breaker rejection rate"
          description: "{{ $labels.bidder }} rejecting >10 req/s due to open circuit"

  - name: auction_health
    rules:
      - alert: HighAuctionLatency
        expr: histogram_quantile(0.95, rate(pbs_auction_duration_seconds_bucket[5m])) > 1.0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High auction latency detected"
          description: "P95 auction latency is {{ $value }}s"

      - alert: LowAuctionSuccessRate
        expr: rate(pbs_auctions_total{status="success"}[5m]) / rate(pbs_auctions_total[5m]) < 0.95
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "Low auction success rate"
          description: "Only {{ $value | humanizePercentage }} auctions succeeding"

  - name: bidder_health
    rules:
      - alert: BidderHighTimeoutRate
        expr: rate(pbs_bidder_timeouts_total[5m]) / rate(pbs_bidder_requests_total[5m]) > 0.10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Bidder timeout rate >10%"
          description: "{{ $labels.bidder }} timeout rate: {{ $value | humanizePercentage }}"

      - alert: BidderDown
        expr: rate(pbs_bidder_errors_total[5m]) / rate(pbs_bidder_requests_total[5m]) > 0.50
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Bidder may be down"
          description: "{{ $labels.bidder }} error rate: {{ $value | humanizePercentage }}"
```

---

## Querying from Command Line

```bash
# Check circuit breaker states
curl -s http://localhost:8000/metrics | grep pbs_bidder_circuit_breaker_state

# Get all circuit breaker metrics
curl -s http://localhost:8000/metrics | grep bidder_circuit

# Export all metrics
curl -s http://localhost:8000/metrics > metrics_snapshot.txt
```

---

## Integration with Monitoring Tools

### Prometheus Configuration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'catalyst'
    scrape_interval: 15s
    static_configs:
      - targets: ['localhost:8000']
```

### Datadog Integration

```python
from datadog import statsd

# Record circuit breaker state change
statsd.increment('catalyst.circuit_breaker.state_change',
                 tags=['bidder:rubicon', 'from:closed', 'to:open'])
```

### New Relic Integration

```go
// Report custom metric
newrelic.RecordCustomMetric(
    "Custom/CircuitBreaker/State",
    float64(state),
)
```

---

## Best Practices

### 1. Monitor Circuit Breaker Health
- Set up alerts for multiple open circuits (>3)
- Monitor state change frequency (flapping)
- Track rejection rates

### 2. Track Auction Performance
- Monitor P95/P99 latency
- Alert on success rate drops
- Correlate latency with bidder health

### 3. Bidder Reliability
- Track timeout rates per bidder
- Monitor circuit breaker activations
- Set SLOs for bidder performance

### 4. Use Histograms for Percentiles
```promql
# Always use histogram_quantile for percentiles
histogram_quantile(0.95, rate(pbs_auction_duration_seconds_bucket[5m]))

# Not: avg(pbs_auction_duration_seconds)
```

### 5. Rate vs Increase
```promql
# Use rate() for per-second rates
rate(pbs_auctions_total[5m])

# Use increase() for total over time window
increase(pbs_auctions_total[1h])
```

---

## Troubleshooting

### Circuit Breaker Stuck Open
```promql
# Find bidders with circuits open >10 minutes
time() - timestamp(pbs_bidder_circuit_breaker_state_changes_total{to_state="open"}) > 600
```

### Identifying Slow Bidders
```promql
# Bidders with P95 latency >150ms
histogram_quantile(0.95, sum by (bidder, le) (rate(pbs_bidder_latency_seconds_bucket[5m]))) > 0.150
```

### Memory Leak Detection
```promql
# Track memory growth
deriv(process_resident_memory_bytes[30m]) > 0
```

---

## Metric Retention

**Recommended retention periods**:
- Raw metrics: 7 days
- 5-minute aggregates: 30 days
- 1-hour aggregates: 1 year

---

## Support

For metric additions or custom dashboards, see:
- `internal/metrics/prometheus.go` - Metric definitions
- `internal/exchange/exchange.go` - Circuit breaker integration
- Deployment guide: `deployment/DEPLOYMENT-CHECKLIST.md`

**Last Updated**: 2026-01-17
**Version**: v2.0 (includes circuit breaker metrics)
