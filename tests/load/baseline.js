/**
 * Baseline Load Test
 *
 * Purpose: Establish baseline performance metrics under steady load
 * Duration: 5 minutes
 * Target: 1k-10k QPS
 *
 * Run: k6 run tests/load/baseline.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const auctionDuration = new Trend('auction_duration');
const bidsReceived = new Counter('bids_received');
const noBidRate = new Rate('no_bid_rate');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 50 },   // Ramp up to 50 VUs
    { duration: '2m', target: 100 },   // Ramp up to 100 VUs
    { duration: '2m', target: 100 },   // Stay at 100 VUs (steady state)
    { duration: '30s', target: 0 },    // Ramp down to 0
  ],
  thresholds: {
    'http_req_duration': ['p(95)<200', 'p(99)<500'],  // 95% < 200ms, 99% < 500ms
    'http_req_failed': ['rate<0.01'],                  // Error rate < 1%
    'errors': ['rate<0.01'],                           // Custom error rate < 1%
    'no_bid_rate': ['rate<0.30'],                      // No-bid rate < 30%
  },
  ext: {
    loadimpact: {
      projectID: 3570770,
      name: 'Baseline Load Test'
    }
  }
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';

// Sample publisher IDs for testing
const PUBLISHER_IDS = [
  'pub-test-001',
  'pub-test-002',
  'pub-test-003',
  'pub-demo-001',
  'pub-demo-002',
];

// Sample domains
const DOMAINS = [
  'example.com',
  'test-publisher.com',
  'demo-site.com',
  'news-example.com',
  'sports-demo.com',
];

// Ad sizes (IAB standard)
const AD_SIZES = [
  { w: 300, h: 250 },   // Medium Rectangle
  { w: 728, h: 90 },    // Leaderboard
  { w: 160, h: 600 },   // Wide Skyscraper
  { w: 320, h: 50 },    // Mobile Banner
  { w: 970, h: 250 },   // Billboard
];

/**
 * Generate a realistic OpenRTB 2.x bid request
 */
function generateBidRequest() {
  const publisherID = PUBLISHER_IDS[Math.floor(Math.random() * PUBLISHER_IDS.length)];
  const domain = DOMAINS[Math.floor(Math.random() * DOMAINS.length)];
  const numImpressions = Math.random() < 0.7 ? 1 : Math.floor(Math.random() * 3) + 1;

  const impressions = [];
  for (let i = 0; i < numImpressions; i++) {
    const size = AD_SIZES[Math.floor(Math.random() * AD_SIZES.length)];
    impressions.push({
      id: `imp-${i + 1}`,
      banner: {
        w: size.w,
        h: size.h,
        pos: Math.random() < 0.5 ? 1 : 0,  // Above/below fold
      },
      bidfloor: Math.random() < 0.5 ? 0.50 : 1.00,
      bidfloorcur: 'USD',
    });
  }

  return {
    id: `req-${__VU}-${__ITER}-${Date.now()}`,
    imp: impressions,
    site: {
      id: domain,
      domain: domain,
      page: `https://${domain}/article/${Math.floor(Math.random() * 10000)}`,
      publisher: {
        id: publisherID,
      },
    },
    device: {
      ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
      ip: `192.168.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}`,
      devicetype: Math.random() < 0.7 ? 2 : 1,  // 70% desktop, 30% mobile
    },
    user: {
      id: `user-${Math.floor(Math.random() * 100000)}`,
    },
    at: 2,  // Second price auction
    tmax: 150,  // 150ms timeout
    cur: ['USD'],
    bcat: ['IAB25', 'IAB26'],  // Blocked categories
  };
}

/**
 * Main test function - executed once per iteration per VU
 */
export default function () {
  const payload = JSON.stringify(generateBidRequest());

  const publisherID = PUBLISHER_IDS[Math.floor(Math.random() * PUBLISHER_IDS.length)];

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'X-Publisher-ID': publisherID,
    },
    timeout: '5s',
  };

  const start = Date.now();
  const response = http.post(`${BASE_URL}/openrtb2/auction`, payload, params);
  const duration = Date.now() - start;

  // Record custom metrics
  auctionDuration.add(duration);

  // Check response
  const success = check(response, {
    'status is 200': (r) => r.status === 200,
    'response is valid JSON': (r) => {
      try {
        JSON.parse(r.body);
        return true;
      } catch (e) {
        return false;
      }
    },
    'has valid response': (r) => {
      try {
        const body = JSON.parse(r.body);
        // Valid response must have 'id' field at minimum
        // May have seatbid (with bids), nbr (no-bid reason), or just id+cur (no bids/no reason)
        return body.hasOwnProperty('id');
      } catch (e) {
        return false;
      }
    },
    'latency under 500ms': (r) => r.timings.duration < 500,
  });

  if (!success) {
    errorRate.add(1);
    console.error(`Request failed: ${response.status} - ${response.body}`);
  } else {
    errorRate.add(0);

    // Track bids received
    try {
      const body = JSON.parse(response.body);
      const hasBids = body.seatbid && body.seatbid.length > 0;

      if (hasBids) {
        let totalBids = 0;
        body.seatbid.forEach(seat => {
          if (seat.bid) {
            totalBids += seat.bid.length;
          }
        });
        bidsReceived.add(totalBids);
        noBidRate.add(0);
      } else {
        noBidRate.add(1);
      }
    } catch (e) {
      console.error(`Failed to parse response: ${e}`);
    }
  }

  // Think time - simulate realistic user behavior
  sleep(Math.random() * 0.5 + 0.1);  // 100-600ms between requests
}

/**
 * Setup - runs once before test starts
 */
export function setup() {
  console.log('🚀 Starting baseline load test');
  console.log(`Target: ${BASE_URL}`);
  console.log('Duration: 5 minutes');
  console.log('Pattern: Ramp 30s → Steady 2m → Ramp down 30s');

  // Health check
  const healthCheck = http.get(`${BASE_URL}/health`);
  if (healthCheck.status !== 200) {
    throw new Error(`Server not ready: ${healthCheck.status}`);
  }

  return { startTime: Date.now() };
}

/**
 * Teardown - runs once after test completes
 */
export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000;
  console.log(`✅ Test completed in ${duration.toFixed(1)}s`);
}

/**
 * Handle summary - custom summary output
 */
export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
    'summary.json': JSON.stringify(data, null, 2),
  };
}

function textSummary(data, options) {
  const indent = options?.indent || '';
  const enableColors = options?.enableColors || false;

  let summary = '\n';
  summary += `${indent}✅ Baseline Load Test Results\n`;
  summary += `${indent}${'='.repeat(50)}\n\n`;

  const metrics = data.metrics;

  // HTTP metrics
  if (metrics.http_reqs) {
    summary += `${indent}Total Requests: ${metrics.http_reqs.values.count}\n`;
    summary += `${indent}Request Rate: ${metrics.http_reqs.values.rate.toFixed(2)}/s\n\n`;
  }

  // Duration metrics
  if (metrics.http_req_duration) {
    summary += `${indent}Latency:\n`;
    summary += `${indent}  avg: ${metrics.http_req_duration.values.avg.toFixed(2)}ms\n`;
    summary += `${indent}  min: ${metrics.http_req_duration.values.min.toFixed(2)}ms\n`;
    summary += `${indent}  med: ${metrics.http_req_duration.values.med.toFixed(2)}ms\n`;
    summary += `${indent}  max: ${metrics.http_req_duration.values.max.toFixed(2)}ms\n`;
    summary += `${indent}  p90: ${metrics.http_req_duration.values['p(90)'].toFixed(2)}ms\n`;
    summary += `${indent}  p95: ${metrics.http_req_duration.values['p(95)'].toFixed(2)}ms\n`;
    summary += `${indent}  p99: ${metrics.http_req_duration.values['p(99)'].toFixed(2)}ms\n\n`;
  }

  // Error rate
  if (metrics.http_req_failed) {
    const failRate = metrics.http_req_failed.values.rate * 100;
    summary += `${indent}Error Rate: ${failRate.toFixed(2)}%\n`;
  }

  // Custom metrics
  if (metrics.no_bid_rate) {
    const noBidPct = metrics.no_bid_rate.values.rate * 100;
    summary += `${indent}No-Bid Rate: ${noBidPct.toFixed(2)}%\n`;
  }

  if (metrics.bids_received) {
    summary += `${indent}Total Bids: ${metrics.bids_received.values.count}\n`;
  }

  summary += `\n${indent}${'='.repeat(50)}\n`;

  return summary;
}
