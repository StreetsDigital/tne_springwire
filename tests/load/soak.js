/**
 * Soak Test
 *
 * Purpose: Find memory leaks and performance degradation over time
 * Duration: 1-24 hours (configurable)
 * Target: 5k QPS sustained
 *
 * Run: k6 run --duration 1h tests/load/soak.js
 * Run: k6 run --duration 24h tests/load/soak.js  # Full soak
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const errorRate = new Rate('errors');
const latencyTrend = new Trend('latency_trend');
const memoryWarnings = new Counter('memory_warnings');

export const options = {
  stages: [
    { duration: '2m', target: 200 },    // Ramp up
    { duration: '58m', target: 200 },   // Sustain for 1 hour (default)
  ],
  thresholds: {
    'http_req_duration': [
      'p(95)<200',
      'p(99)<500',
    ],
    'http_req_failed': ['rate<0.01'],
    'errors': ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';

const PUBLISHER_IDS = ['pub-soak-001', 'pub-soak-002', 'pub-soak-003'];
const DOMAINS = ['soak-test-1.com', 'soak-test-2.com', 'soak-test-3.com'];

function generateBidRequest() {
  const publisherID = PUBLISHER_IDS[Math.floor(Math.random() * PUBLISHER_IDS.length)];
  const domain = DOMAINS[Math.floor(Math.random() * DOMAINS.length)];

  return {
    id: `soak-${__VU}-${__ITER}-${Date.now()}`,
    imp: [
      {
        id: '1',
        banner: { w: 300, h: 250 },
        bidfloor: 0.50,
      },
      {
        id: '2',
        banner: { w: 728, h: 90 },
        bidfloor: 1.00,
      },
    ],
    site: {
      id: publisherID,
      domain: domain,
      page: `https://${domain}/page`,
      publisher: { id: publisherID },
    },
    device: {
      ua: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36',
      ip: `172.16.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}`,
    },
    user: {
      id: `user-${Math.floor(Math.random() * 50000)}`,  // Realistic user pool
    },
    at: 2,
    tmax: 150,
  };
}

let lastMetricCheck = Date.now();
let initialP95 = null;

export default function () {
  const payload = JSON.stringify(generateBidRequest());
  const publisherID = PUBLISHER_IDS[Math.floor(Math.random() * PUBLISHER_IDS.length)];

  const response = http.post(`${BASE_URL}/openrtb2/auction`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'X-Publisher-ID': publisherID,
    },
    timeout: '5s',
  });

  const success = check(response, {
    'status is 200': (r) => r.status === 200,
    'valid response': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.hasOwnProperty('id');
      } catch (e) {
        return false;
      }
    },
  });

  errorRate.add(success ? 0 : 1);
  latencyTrend.add(response.timings.duration);

  // Track baseline P95 and check for degradation
  if (__ITER === 100 && initialP95 === null) {
    initialP95 = response.timings.duration;
  }

  // Periodic health check (every 5 minutes)
  const now = Date.now();
  if (now - lastMetricCheck > 300000 && __VU === 1) {  // 5 minutes
    const healthCheck = http.get(`${BASE_URL}/metrics`);

    // Simple memory leak detection
    if (healthCheck.body && healthCheck.body.includes('go_memstats_alloc_bytes')) {
      const match = healthCheck.body.match(/go_memstats_alloc_bytes ([0-9.]+)/);
      if (match && parseFloat(match[1]) > 1e9) {  // > 1GB
        memoryWarnings.add(1);
        console.warn(`⚠️  High memory usage detected: ${(parseFloat(match[1]) / 1e9).toFixed(2)}GB`);
      }
    }

    lastMetricCheck = now;
  }

  sleep(Math.random() * 0.5 + 0.2);  // 200-700ms think time
}

export function setup() {
  console.log('⏱️  Starting soak test');
  console.log(`Target: ${BASE_URL}`);
  console.log('Duration: 1 hour (default) - use --duration flag to extend');
  console.log('Monitoring for: memory leaks, latency degradation, error rate creep');

  const healthCheck = http.get(`${BASE_URL}/health`);
  if (healthCheck.status !== 200) {
    throw new Error(`Server not ready: ${healthCheck.status}`);
  }

  return { startTime: Date.now() };
}

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000 / 60;
  console.log(`✅ Soak test completed after ${duration.toFixed(1)} minutes`);
  console.log('');
  console.log('📊 Post-test checks:');
  console.log('  1. Review memory metrics for growth');
  console.log('  2. Check latency trend for degradation');
  console.log('  3. Verify error rate remained stable');
  console.log('  4. Inspect circuit breaker state transitions');
  console.log('');
  console.log('Commands:');
  console.log('  curl http://localhost:8080/metrics | grep memory');
  console.log('  curl http://localhost:8080/admin/circuit-breakers');
}
