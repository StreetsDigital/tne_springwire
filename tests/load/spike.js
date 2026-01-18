/**
 * Spike Load Test
 *
 * Purpose: Test system behavior under sudden traffic spikes
 * Duration: 2 minutes
 * Target: 50k QPS burst
 *
 * Run: k6 run tests/load/spike.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const recoveryTime = new Trend('recovery_time');

export const options = {
  stages: [
    { duration: '10s', target: 50 },    // Normal load
    { duration: '10s', target: 1000 },  // SPIKE! 20x increase
    { duration: '30s', target: 1000 },  // Sustain spike
    { duration: '10s', target: 50 },    // Return to normal
    { duration: '20s', target: 50 },    // Monitor recovery
  ],
  thresholds: {
    'http_req_duration': ['p(99)<2000'],  // Relaxed during spike
    'http_req_failed': ['rate<0.05'],     // Allow 5% errors during spike
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';

function generateBidRequest() {
  return {
    id: `spike-${__VU}-${__ITER}-${Date.now()}`,
    imp: [{
      id: '1',
      banner: { w: 300, h: 250 },
      bidfloor: 0.50,
    }],
    site: {
      id: 'pub-spike-test',
      domain: 'spike-test.com',
      publisher: {
        id: 'pub-spike-test',
      },
    },
    device: {
      ip: `10.0.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}`,
    },
    at: 2,
    tmax: 200,
  };
}

export default function () {
  const payload = JSON.stringify(generateBidRequest());
  const response = http.post(`${BASE_URL}/openrtb2/auction`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'X-Publisher-ID': 'pub-spike-test',
    },
  });

  const success = check(response, {
    'status is 200': (r) => r.status === 200,
    'latency acceptable': (r) => r.timings.duration < 2000,
  });

  errorRate.add(success ? 0 : 1);

  sleep(0.01);  // Minimal think time for spike
}

export function setup() {
  console.log('⚡ Starting spike test');
  console.log('Pattern: Normal → 20x SPIKE → Sustain → Recovery');
}

export function teardown() {
  console.log('✅ Spike test completed');
  console.log('💡 Check circuit breaker stats and error rates');
}
