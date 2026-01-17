package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ============================================================================
// METRICS RECORDING OVERHEAD BENCHMARKS
// ============================================================================

// BenchmarkMetrics_RecordBid benchmarks bid recording overhead
func BenchmarkMetrics_RecordBid(b *testing.B) {
	m := createTestMetricsWithAll("bench_bid")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordBid("rubicon", "banner", 1.50)
	}
}

// BenchmarkMetrics_RecordAuction benchmarks auction recording overhead
func BenchmarkMetrics_RecordAuction(b *testing.B) {
	m := createTestMetricsWithAll("bench_auction")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordAuction("success", "banner", 150*time.Millisecond, 5, 2)
	}
}

// BenchmarkMetrics_RecordBidderRequest benchmarks bidder request recording
func BenchmarkMetrics_RecordBidderRequest(b *testing.B) {
	m := createTestMetricsWithAll("bench_bidder_req")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordBidderRequest("rubicon", 50*time.Millisecond, false, false)
	}
}

// BenchmarkMetrics_RecordMargin benchmarks margin/revenue recording
func BenchmarkMetrics_RecordMargin(b *testing.B) {
	m := createTestMetricsWithAll("bench_margin")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordMargin("pub123", "rubicon", "banner", 2.00, 1.67, 0.33)
	}
}

// ============================================================================
// CIRCUIT BREAKER METRICS OVERHEAD BENCHMARKS
// ============================================================================

// BenchmarkMetrics_SetBidderCircuitState benchmarks circuit state gauge updates
func BenchmarkMetrics_SetBidderCircuitState(b *testing.B) {
	m := createTestMetricsWithAll("bench_circuit_state")

	states := []string{"closed", "open", "half-open"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.SetBidderCircuitState("bidder1", states[i%3])
	}
}

// BenchmarkMetrics_RecordBidderCircuitRequest benchmarks circuit request counting
func BenchmarkMetrics_RecordBidderCircuitRequest(b *testing.B) {
	m := createTestMetricsWithAll("bench_circuit_req")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordBidderCircuitRequest("bidder1")
	}
}

// BenchmarkMetrics_RecordBidderCircuitFailure benchmarks circuit failure recording
func BenchmarkMetrics_RecordBidderCircuitFailure(b *testing.B) {
	m := createTestMetricsWithAll("bench_circuit_fail")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordBidderCircuitFailure("bidder1")
	}
}

// BenchmarkMetrics_RecordBidderCircuitSuccess benchmarks circuit success recording
func BenchmarkMetrics_RecordBidderCircuitSuccess(b *testing.B) {
	m := createTestMetricsWithAll("bench_circuit_success")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordBidderCircuitSuccess("bidder1")
	}
}

// BenchmarkMetrics_RecordBidderCircuitRejected benchmarks circuit rejection recording
func BenchmarkMetrics_RecordBidderCircuitRejected(b *testing.B) {
	m := createTestMetricsWithAll("bench_circuit_reject")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordBidderCircuitRejected("bidder1")
	}
}

// BenchmarkMetrics_RecordBidderCircuitStateChange benchmarks state transition recording
func BenchmarkMetrics_RecordBidderCircuitStateChange(b *testing.B) {
	m := createTestMetricsWithAll("bench_circuit_change")

	transitions := []struct{ from, to string }{
		{"closed", "open"},
		{"open", "half-open"},
		{"half-open", "closed"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t := transitions[i%3]
		m.RecordBidderCircuitStateChange("bidder1", t.from, t.to)
	}
}

// ============================================================================
// COMBINED OPERATIONS BENCHMARKS
// ============================================================================

// BenchmarkMetrics_RealisticAuctionScenario benchmarks realistic metric recording pattern
func BenchmarkMetrics_RealisticAuctionScenario(b *testing.B) {
	m := createTestMetricsWithAll("bench_realistic")

	bidders := []string{"rubicon", "appnexus", "pubmatic", "openx", "ix"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Auction start
		for _, bidder := range bidders {
			// Check circuit breaker
			m.RecordBidderCircuitRequest(bidder)

			// Record bidder call
			m.RecordBidderRequest(bidder, 50*time.Millisecond, false, false)

			// Record success
			m.RecordBidderCircuitSuccess(bidder)

			// Record bid
			m.RecordBid(bidder, "banner", 1.50)
		}

		// Record auction completion
		m.RecordAuction("success", "banner", 150*time.Millisecond, 5, 0)
	}
}

// BenchmarkMetrics_RealisticFailureScenario benchmarks metric recording during failures
func BenchmarkMetrics_RealisticFailureScenario(b *testing.B) {
	m := createTestMetricsWithAll("bench_failure")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Bidder fails
		m.RecordBidderCircuitRequest("failing_bidder")
		m.RecordBidderRequest("failing_bidder", 100*time.Millisecond, true, true)
		m.RecordBidderCircuitFailure("failing_bidder")

		// Circuit opens after 5 failures
		if i%5 == 0 {
			m.SetBidderCircuitState("failing_bidder", "open")
			m.RecordBidderCircuitStateChange("failing_bidder", "closed", "open")
		}

		// Rejected requests while open
		if i%5 != 0 {
			m.RecordBidderCircuitRejected("failing_bidder")
		}
	}
}

// ============================================================================
// PROMETHEUS PRIMITIVE BENCHMARKS (FOR COMPARISON)
// ============================================================================

// BenchmarkPrometheus_Counter benchmarks raw counter increment
func BenchmarkPrometheus_Counter(b *testing.B) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "bench_counter",
		Help: "Benchmark counter",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Inc()
	}
}

// BenchmarkPrometheus_CounterVec benchmarks counter with labels
func BenchmarkPrometheus_CounterVec(b *testing.B) {
	counterVec := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bench_counter_vec",
			Help: "Benchmark counter vec",
		},
		[]string{"bidder", "type"},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counterVec.WithLabelValues("rubicon", "banner").Inc()
	}
}

// BenchmarkPrometheus_Gauge benchmarks raw gauge set
func BenchmarkPrometheus_Gauge(b *testing.B) {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "bench_gauge",
		Help: "Benchmark gauge",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gauge.Set(float64(i % 3))
	}
}

// BenchmarkPrometheus_GaugeVec benchmarks gauge with labels
func BenchmarkPrometheus_GaugeVec(b *testing.B) {
	gaugeVec := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bench_gauge_vec",
			Help: "Benchmark gauge vec",
		},
		[]string{"bidder"},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gaugeVec.WithLabelValues("rubicon").Set(float64(i % 3))
	}
}

// BenchmarkPrometheus_Histogram benchmarks histogram observe
func BenchmarkPrometheus_Histogram(b *testing.B) {
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "bench_histogram",
		Help:    "Benchmark histogram",
		Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		histogram.Observe(0.150)
	}
}

// BenchmarkPrometheus_HistogramVec benchmarks histogram with labels
func BenchmarkPrometheus_HistogramVec(b *testing.B) {
	histogramVec := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bench_histogram_vec",
			Help:    "Benchmark histogram vec",
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"bidder"},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		histogramVec.WithLabelValues("rubicon").Observe(0.150)
	}
}

// ============================================================================
// CONCURRENT METRICS BENCHMARKS
// ============================================================================

// BenchmarkMetrics_Concurrent_CircuitBreaker benchmarks concurrent circuit breaker metrics
func BenchmarkMetrics_Concurrent_CircuitBreaker(b *testing.B) {
	m := createTestMetricsWithAll("bench_concurrent")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bidder := "bidder" + string(rune('1'+i%5))
			m.RecordBidderCircuitRequest(bidder)
			m.RecordBidderCircuitSuccess(bidder)
			i++
		}
	})
}

// BenchmarkMetrics_Concurrent_AuctionRecording benchmarks concurrent auction recording
func BenchmarkMetrics_Concurrent_AuctionRecording(b *testing.B) {
	m := createTestMetricsWithAll("bench_concurrent_auction")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.RecordAuction("success", "banner", 100*time.Millisecond, 5, 2)
		}
	})
}
