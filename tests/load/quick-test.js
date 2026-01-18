/**
 * Quick Load Test - 2 minutes
 *
 * Purpose: Quick validation that load testing works
 * Run: k6 run tests/load/quick-test.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 10 },   // Warm up
    { duration: '1m', target: 50 },    // Test at 50 VUs
    { duration: '30s', target: 0 },    // Cool down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<300'],
    'http_req_failed': ['rate<0.05'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';

export default function () {
  const payload = JSON.stringify({
    id: `quick-${__VU}-${__ITER}`,
    imp: [{
      id: '1',
      banner: { w: 300, h: 250 },
      bidfloor: 0.50,
    }],
    site: {
      id: 'pub-quick-test',
      domain: 'quick-test.com',
      publisher: {
        id: 'pub-quick-test',
      },
    },
    at: 2,
    tmax: 150,
  });

  const res = http.post(`${BASE_URL}/openrtb2/auction`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'X-Publisher-ID': 'pub-quick-test',
    },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
    'latency OK': (r) => r.timings.duration < 300,
  });

  sleep(0.5);
}

export function setup() {
  console.log('🚀 Quick 2-minute validation test');
  console.log(`Target: ${BASE_URL}`);
}

export function teardown() {
  console.log('✅ Quick test complete!');
  console.log('If successful, proceed to: make load-baseline');
}
