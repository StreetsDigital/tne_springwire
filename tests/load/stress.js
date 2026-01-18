/**
 * Stress Test
 *
 * Purpose: Find the breaking point - maximum capacity
 * Duration: 10 minutes
 * Target: Ramp until failure
 *
 * Run: k6 run tests/load/stress.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const errorRate = new Rate('errors');
const breakingPoint = new Counter('breaking_point_reached');

export const options = {
  stages: [
    { duration: '1m', target: 100 },    // Warm up
    { duration: '2m', target: 500 },    // Moderate load
    { duration: '2m', target: 1000 },   // High load
    { duration: '2m', target: 2000 },   // Very high load
    { duration: '2m', target: 5000 },   // Extreme load
    { duration: '1m', target: 10000 },  // Breaking point
  ],
  thresholds: {
    // No thresholds - expect failure
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';

function generateBidRequest() {
  return {
    id: `stress-${__VU}-${__ITER}`,
    imp: [{
      id: '1',
      banner: { w: 300, h: 250 },
    }],
    site: {
      id: 'pub-stress-test',
      domain: 'stress.example.com',
      publisher: {
        id: 'pub-stress-test',
      },
    },
    at: 2,
    tmax: 200,
  };
}

let lastErrorRate = 0;

export default function () {
  const payload = JSON.stringify(generateBidRequest());

  const response = http.post(`${BASE_URL}/openrtb2/auction`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'X-Publisher-ID': 'pub-stress-test',
    },
    timeout: '3s',
  });

  const success = check(response, {
    'status is 200': (r) => r.status === 200,
    'responded in time': (r) => r.status !== 0,  // Not timeout
  });

  const currentErrorRate = success ? 0 : 1;
  errorRate.add(currentErrorRate);

  // Detect breaking point (error rate > 50% sustained)
  if (__VU === 1 && __ITER % 100 === 0) {
    const stats = errorRate.rate || 0;
    if (stats > 0.5 && lastErrorRate > 0.5) {
      breakingPoint.add(1);
      console.warn(`🔥 Breaking point reached! Error rate: ${(stats * 100).toFixed(1)}% at ${__VU} VUs`);
    }
    lastErrorRate = stats;
  }

  sleep(0.01);  // Minimal sleep - maximize load
}

export function setup() {
  console.log('💥 Starting stress test');
  console.log('Purpose: Find the breaking point');
  console.log('Pattern: Ramp 100 → 500 → 1k → 2k → 5k → 10k VUs');
  console.log('Expected outcome: System will fail at some point - that\'s OK!');
}

export function teardown() {
  console.log('✅ Stress test completed');
  console.log('');
  console.log('📊 Analysis:');
  console.log('  1. At what VU count did errors spike?');
  console.log('  2. What was the maximum sustained throughput?');
  console.log('  3. Did the system recover after load decreased?');
  console.log('  4. Were circuit breakers activated?');
}
