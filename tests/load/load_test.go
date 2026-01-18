// +build loadtest

package load

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

var (
	endpoint = flag.String("endpoint", "http://localhost:8080/openrtb2/auction", "Auction endpoint URL")
	qps      = flag.Int("qps", 1000, "Target queries per second")
	duration = flag.Duration("duration", 5*time.Minute, "Test duration")
	workers  = flag.Int("workers", 100, "Number of concurrent workers")
)

// Stats tracks load test metrics
type Stats struct {
	TotalRequests  atomic.Int64
	SuccessCount   atomic.Int64
	ErrorCount     atomic.Int64
	TimeoutCount   atomic.Int64
	TotalLatencyMs atomic.Int64

	// Latency buckets (ms)
	Under50ms   atomic.Int64
	Under100ms  atomic.Int64
	Under200ms  atomic.Int64
	Under500ms  atomic.Int64
	Under1000ms atomic.Int64
	Over1000ms  atomic.Int64

	startTime time.Time
	mu        sync.Mutex
	latencies []time.Duration // For percentile calculation
}

// TestLoadBaseline runs a baseline load test
func TestLoadBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	runLoadTest(t, &LoadTestConfig{
		Name:     "Baseline",
		QPS:      *qps,
		Duration: *duration,
		Workers:  *workers,
	})
}

// TestLoadSpike runs a spike test
func TestLoadSpike(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping spike test in short mode")
	}

	// Spike pattern: 1k → 10k → 1k QPS
	t.Log("Running spike test: 1k → 10k → 1k QPS")

	runLoadTest(t, &LoadTestConfig{
		Name:     "Spike",
		QPS:      1000,
		Duration: 30 * time.Second,
		Workers:  100,
	})

	t.Log("SPIKE! Increasing to 10k QPS")
	runLoadTest(t, &LoadTestConfig{
		Name:     "Spike-Peak",
		QPS:      10000,
		Duration: 30 * time.Second,
		Workers:  1000,
	})

	t.Log("Recovery: Back to 1k QPS")
	runLoadTest(t, &LoadTestConfig{
		Name:     "Spike-Recovery",
		QPS:      1000,
		Duration: 30 * time.Second,
		Workers:  100,
	})
}

// TestLoadSoak runs a soak test
func TestLoadSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping soak test in short mode")
	}

	// Use environment or default to 1 hour
	soakDuration := *duration
	if soakDuration < 1*time.Hour {
		soakDuration = 1 * time.Hour
		t.Logf("Extending soak test to 1 hour (override with -duration flag)")
	}

	runLoadTest(t, &LoadTestConfig{
		Name:     "Soak",
		QPS:      5000,
		Duration: soakDuration,
		Workers:  200,
	})
}

// LoadTestConfig defines load test parameters
type LoadTestConfig struct {
	Name     string
	QPS      int
	Duration time.Duration
	Workers  int
}

// runLoadTest executes a load test with the given configuration
func runLoadTest(t *testing.T, config *LoadTestConfig) {
	t.Helper()

	stats := &Stats{
		startTime: time.Now(),
		latencies: make([]time.Duration, 0, config.QPS*int(config.Duration.Seconds())),
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)
	defer cancel()

	// Create HTTP client with connection pooling
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        config.Workers,
			MaxIdleConnsPerHost: config.Workers,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Rate limiter channel
	ticker := time.NewTicker(time.Second / time.Duration(config.QPS))
	defer ticker.Stop()

	// Worker pool
	var wg sync.WaitGroup
	requestChan := make(chan struct{}, config.Workers*2)

	// Start workers
	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			worker(ctx, client, *endpoint, stats, requestChan)
		}(i)
	}

	// Progress reporter
	progressTicker := time.NewTicker(10 * time.Second)
	defer progressTicker.Stop()

	go func() {
		for {
			select {
			case <-progressTicker.C:
				printProgress(t, stats, config)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Send requests at target QPS
	t.Logf("🚀 Starting %s load test: %d QPS for %s with %d workers",
		config.Name, config.QPS, config.Duration, config.Workers)

	for {
		select {
		case <-ctx.Done():
			close(requestChan)
			wg.Wait()
			printFinalReport(t, stats, config)
			return
		case <-ticker.C:
			select {
			case requestChan <- struct{}{}:
			default:
				// Worker pool full, skip this tick
				stats.ErrorCount.Add(1)
			}
		}
	}
}

// worker processes auction requests
func worker(ctx context.Context, client *http.Client, endpoint string, stats *Stats, requests <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-requests:
			if !ok {
				return
			}
			sendRequest(client, endpoint, stats)
		}
	}
}

// sendRequest sends a single auction request
func sendRequest(client *http.Client, endpoint string, stats *Stats) {
	stats.TotalRequests.Add(1)

	// Generate realistic bid request
	bidRequest := generateBidRequest()
	payload, err := json.Marshal(bidRequest)
	if err != nil {
		stats.ErrorCount.Add(1)
		return
	}

	// Send request
	start := time.Now()
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		stats.ErrorCount.Add(1)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Publisher-ID", bidRequest.Site.ID)

	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		stats.ErrorCount.Add(1)
		stats.TimeoutCount.Add(1)
		return
	}
	defer resp.Body.Close()

	// Read response
	_, _ = io.Copy(io.Discard, resp.Body)

	// Record metrics
	if resp.StatusCode == http.StatusOK {
		stats.SuccessCount.Add(1)
	} else {
		stats.ErrorCount.Add(1)
	}

	// Record latency
	latencyMs := latency.Milliseconds()
	stats.TotalLatencyMs.Add(latencyMs)

	switch {
	case latencyMs < 50:
		stats.Under50ms.Add(1)
	case latencyMs < 100:
		stats.Under100ms.Add(1)
	case latencyMs < 200:
		stats.Under200ms.Add(1)
	case latencyMs < 500:
		stats.Under500ms.Add(1)
	case latencyMs < 1000:
		stats.Under1000ms.Add(1)
	default:
		stats.Over1000ms.Add(1)
	}

	// Store latency for percentile calculation
	stats.mu.Lock()
	stats.latencies = append(stats.latencies, latency)
	stats.mu.Unlock()
}

// generateBidRequest creates a realistic OpenRTB bid request
func generateBidRequest() *openrtb.BidRequest {
	publisherIDs := []string{"pub-test-001", "pub-test-002", "pub-test-003"}
	domains := []string{"example.com", "test.com", "demo.com"}

	pubID := publisherIDs[rand.Intn(len(publisherIDs))]
	domain := domains[rand.Intn(len(domains))]

	return &openrtb.BidRequest{
		ID: fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), rand.Intn(100000)),
		Imp: []openrtb.Imp{
			{
				ID:       "1",
				Banner:   &openrtb.Banner{W: 300, H: 250},
				BidFloor: 0.50,
			},
		},
		Site: &openrtb.Site{
			ID:     pubID,
			Domain: domain,
			Page:   fmt.Sprintf("https://%s/page-%d", domain, rand.Intn(1000)),
		},
		Device: &openrtb.Device{
			UA: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
			IP: fmt.Sprintf("192.168.%d.%d", rand.Intn(256), rand.Intn(256)),
		},
		User: &openrtb.User{
			ID: fmt.Sprintf("user-%d", rand.Intn(100000)),
		},
		AT:   2,
		TMax: 150,
	}
}

// printProgress prints intermediate results
func printProgress(t *testing.T, stats *Stats, config *LoadTestConfig) {
	t.Helper()

	elapsed := time.Since(stats.startTime)
	total := stats.TotalRequests.Load()
	success := stats.SuccessCount.Load()
	errors := stats.ErrorCount.Load()

	if total == 0 {
		return
	}

	actualQPS := float64(total) / elapsed.Seconds()
	successRate := float64(success) / float64(total) * 100
	avgLatency := float64(stats.TotalLatencyMs.Load()) / float64(total)

	t.Logf("⏱️  [%s] Requests: %d | QPS: %.0f | Success: %.1f%% | Avg Latency: %.1fms | Errors: %d",
		elapsed.Truncate(time.Second), total, actualQPS, successRate, avgLatency, errors)
}

// printFinalReport prints comprehensive final results
func printFinalReport(t *testing.T, stats *Stats, config *LoadTestConfig) {
	t.Helper()

	elapsed := time.Since(stats.startTime)
	total := stats.TotalRequests.Load()
	success := stats.SuccessCount.Load()
	errors := stats.ErrorCount.Load()
	timeouts := stats.TimeoutCount.Load()

	if total == 0 {
		t.Log("No requests completed")
		return
	}

	actualQPS := float64(total) / elapsed.Seconds()
	successRate := float64(success) / float64(total) * 100
	errorRate := float64(errors) / float64(total) * 100
	avgLatency := float64(stats.TotalLatencyMs.Load()) / float64(total)

	// Calculate percentiles
	p50, p95, p99 := calculatePercentiles(stats)

	t.Log("\n" + strings.Repeat("=", 70))
	t.Logf("✅ %s Load Test Results", config.Name)
	t.Log(strings.Repeat("=", 70))
	t.Logf("Duration:        %s", elapsed.Truncate(time.Second))
	t.Logf("Total Requests:  %d", total)
	t.Logf("Successful:      %d (%.2f%%)", success, successRate)
	t.Logf("Errors:          %d (%.2f%%)", errors, errorRate)
	t.Logf("Timeouts:        %d", timeouts)
	t.Logf("")
	t.Logf("Target QPS:      %d", config.QPS)
	t.Logf("Actual QPS:      %.1f", actualQPS)
	t.Logf("")
	t.Logf("Latency (ms):")
	t.Logf("  Average:       %.1f", avgLatency)
	t.Logf("  P50:           %.1f", p50)
	t.Logf("  P95:           %.1f", p95)
	t.Logf("  P99:           %.1f", p99)
	t.Logf("")
	t.Logf("Latency Distribution:")
	t.Logf("  < 50ms:        %d (%.1f%%)", stats.Under50ms.Load(), percent(stats.Under50ms.Load(), total))
	t.Logf("  < 100ms:       %d (%.1f%%)", stats.Under100ms.Load(), percent(stats.Under100ms.Load(), total))
	t.Logf("  < 200ms:       %d (%.1f%%)", stats.Under200ms.Load(), percent(stats.Under200ms.Load(), total))
	t.Logf("  < 500ms:       %d (%.1f%%)", stats.Under500ms.Load(), percent(stats.Under500ms.Load(), total))
	t.Logf("  < 1000ms:      %d (%.1f%%)", stats.Under1000ms.Load(), percent(stats.Under1000ms.Load(), total))
	t.Logf("  > 1000ms:      %d (%.1f%%)", stats.Over1000ms.Load(), percent(stats.Over1000ms.Load(), total))
	t.Log(strings.Repeat("=", 70))

	// Pass/fail criteria
	if successRate < 99.0 {
		t.Errorf("❌ FAIL: Success rate %.2f%% < 99%%", successRate)
	}
	if p95 > 200 {
		t.Errorf("❌ FAIL: P95 latency %.1fms > 200ms", p95)
	}
	if p99 > 500 {
		t.Errorf("❌ FAIL: P99 latency %.1fms > 500ms", p99)
	}
}

// calculatePercentiles calculates P50, P95, P99 from latency samples
func calculatePercentiles(stats *Stats) (p50, p95, p99 float64) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	if len(stats.latencies) == 0 {
		return 0, 0, 0
	}

	// Sort latencies
	latencies := make([]time.Duration, len(stats.latencies))
	copy(latencies, stats.latencies)

	// Simple bubble sort (good enough for percentiles)
	for i := 0; i < len(latencies); i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	p50Idx := int(float64(len(latencies)) * 0.50)
	p95Idx := int(float64(len(latencies)) * 0.95)
	p99Idx := int(float64(len(latencies)) * 0.99)

	return float64(latencies[p50Idx].Milliseconds()),
		float64(latencies[p95Idx].Milliseconds()),
		float64(latencies[p99Idx].Milliseconds())
}

func percent(count, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) / float64(total) * 100
}
