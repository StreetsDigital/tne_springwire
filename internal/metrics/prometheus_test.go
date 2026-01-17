package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Global metrics instance to avoid registry conflicts
var testMetrics *Metrics

func init() {
	// Create a custom registry for tests to avoid conflicts
	testMetrics = createTestMetrics()
}

// createTestMetrics creates metrics with a unique namespace
func createTestMetrics() *Metrics {
	namespace := "test_pbs"

	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		RequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Number of HTTP requests currently being served",
			},
		),
		RateLimitRejected: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_rejected_total",
				Help:      "Total number of requests rejected by rate limiting",
			},
		),
		AuthFailures: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "auth_failures_total",
				Help:      "Total number of authentication failures",
			},
		),
		RevenueTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "revenue_total",
				Help:      "Total revenue from bids",
			},
			[]string{"publisher", "bidder", "media_type"},
		),
		PublisherPayoutTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "publisher_payout_total",
				Help:      "Total payout to publishers",
			},
			[]string{"publisher", "bidder", "media_type"},
		),
		PlatformMarginTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "platform_margin_total",
				Help:      "Total platform margin",
			},
			[]string{"publisher", "bidder", "media_type"},
		),
		MarginPercentage: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "margin_percentage",
				Help:      "Margin percentage distribution",
				Buckets:   []float64{0, 5, 10, 15, 20, 25, 30, 40, 50},
			},
			[]string{"publisher"},
		),
		FloorAdjustments: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "floor_adjustments_total",
				Help:      "Total floor adjustments",
			},
			[]string{"publisher"},
		),
	}

	return m
}

func TestIncRateLimitRejected(t *testing.T) {
	m := testMetrics
	initialValue := testutil.ToFloat64(m.RateLimitRejected)
	
	m.IncRateLimitRejected()
	
	newValue := testutil.ToFloat64(m.RateLimitRejected)
	if newValue != initialValue+1 {
		t.Errorf("Expected rate limit rejected to be %f, got %f", initialValue+1, newValue)
	}
}

func TestIncAuthFailures(t *testing.T) {
	m := testMetrics
	initialValue := testutil.ToFloat64(m.AuthFailures)
	
	m.IncAuthFailures()
	
	newValue := testutil.ToFloat64(m.AuthFailures)
	if newValue != initialValue+1 {
		t.Errorf("Expected auth failures to be %f, got %f", initialValue+1, newValue)
	}
}

func TestRecordMargin(t *testing.T) {
	m := testMetrics
	
	publisher := "pub123"
	bidder := "appnexus"
	mediaType := "banner"
	originalPrice := 2.50
	adjustedPrice := 2.00
	platformCut := 0.50
	
	m.RecordMargin(publisher, bidder, mediaType, originalPrice, adjustedPrice, platformCut)
	
	revenueValue := testutil.ToFloat64(m.RevenueTotal.WithLabelValues(publisher, bidder, mediaType))
	if revenueValue < originalPrice {
		t.Errorf("Expected revenue to include %f, got %f", originalPrice, revenueValue)
	}
}

func TestRecordMargin_ZeroPrice(t *testing.T) {
	m := testMetrics
	
	m.RecordMargin("pub", "bidder", "banner", 0.0, 0.0, 0.0)
	
	// Should not panic
}

func TestRecordFloorAdjustment(t *testing.T) {
	m := testMetrics
	
	publisher := "pub_test"
	initialValue := testutil.ToFloat64(m.FloorAdjustments.WithLabelValues(publisher))
	
	m.RecordFloorAdjustment(publisher)
	
	newValue := testutil.ToFloat64(m.FloorAdjustments.WithLabelValues(publisher))
	if newValue != initialValue+1 {
		t.Errorf("Expected floor adjustments to be %f, got %f", initialValue+1, newValue)
	}
}

func TestMiddleware(t *testing.T) {
	m := testMetrics
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	wrapped := m.Middleware(handler)
	
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	
	wrapped.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestMiddleware_InFlight(t *testing.T) {
	m := testMetrics
	
	initialInFlight := testutil.ToFloat64(m.RequestsInFlight)
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inFlightDuring := testutil.ToFloat64(m.RequestsInFlight)
		if inFlightDuring <= initialInFlight {
			t.Errorf("Expected in-flight to increase during request")
		}
		w.WriteHeader(http.StatusOK)
	})
	
	wrapped := m.Middleware(handler)
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	
	wrapped.ServeHTTP(rr, req)
	
	finalInFlight := testutil.ToFloat64(m.RequestsInFlight)
	if finalInFlight != initialInFlight {
		t.Errorf("Expected in-flight to return to %f, got %f", initialInFlight, finalInFlight)
	}
}

func TestHandler(t *testing.T) {
	handler := Handler()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if len(body) == 0 {
		t.Error("Expected non-empty metrics response")
	}

	if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") {
		t.Error("Expected Prometheus format metrics output")
	}
}

// createTestMetricsWithAll creates metrics with all fields for comprehensive testing
func createTestMetricsWithAll(namespace string) *Metrics {
	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		RequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Number of HTTP requests currently being served",
			},
		),
		AuctionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "auctions_total",
				Help:      "Total number of auctions",
			},
			[]string{"status", "media_type"},
		),
		AuctionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "auction_duration_seconds",
				Help:      "Auction duration in seconds",
				Buckets:   []float64{.01, .025, .05, .1, .25, .5, .75, 1, 1.5, 2},
			},
			[]string{"media_type"},
		),
		BidsReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bids_received_total",
				Help:      "Total number of bids received",
			},
			[]string{"bidder", "media_type"},
		),
		BidCPM: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "bid_cpm",
				Help:      "Bid CPM distribution",
				Buckets:   []float64{0.1, 0.5, 1, 2, 3, 5, 10, 20, 50},
			},
			[]string{"bidder", "media_type"},
		),
		BiddersSelected: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "bidders_selected",
				Help:      "Number of bidders selected per auction",
				Buckets:   []float64{1, 2, 3, 5, 7, 10, 15, 20, 30},
			},
			[]string{"media_type"},
		),
		BiddersExcluded: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "bidders_excluded",
				Help:      "Number of bidders excluded per auction",
			},
			[]string{"reason"},
		),
		BidderRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bidder_requests_total",
				Help:      "Total requests to each bidder",
			},
			[]string{"bidder"},
		),
		BidderLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "bidder_latency_seconds",
				Help:      "Bidder response latency in seconds",
				Buckets:   []float64{.01, .025, .05, .1, .15, .2, .3, .5, .75, 1},
			},
			[]string{"bidder"},
		),
		BidderErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bidder_errors_total",
				Help:      "Total errors from bidders",
			},
			[]string{"bidder", "error_type"},
		),
		BidderTimeouts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bidder_timeouts_total",
				Help:      "Total timeouts from bidders",
			},
			[]string{"bidder"},
		),
		BidderCircuitState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "bidder_circuit_breaker_state",
				Help:      "Bidder circuit breaker state (0=closed, 1=open, 2=half-open)",
			},
			[]string{"bidder"},
		),
		BidderCircuitRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bidder_circuit_breaker_requests_total",
				Help:      "Total requests through bidder circuit breaker",
			},
			[]string{"bidder"},
		),
		BidderCircuitFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bidder_circuit_breaker_failures_total",
				Help:      "Total failures recorded by bidder circuit breaker",
			},
			[]string{"bidder"},
		),
		BidderCircuitSuccesses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bidder_circuit_breaker_successes_total",
				Help:      "Total successes recorded by bidder circuit breaker",
			},
			[]string{"bidder"},
		),
		BidderCircuitRejected: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bidder_circuit_breaker_rejected_total",
				Help:      "Total requests rejected by bidder circuit breaker (circuit open)",
			},
			[]string{"bidder"},
		),
		BidderCircuitStateChanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bidder_circuit_breaker_state_changes_total",
				Help:      "Total circuit breaker state changes",
			},
			[]string{"bidder", "from_state", "to_state"},
		),
		IDRRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "idr_requests_total",
				Help:      "Total requests to IDR service",
			},
			[]string{"status"},
		),
		IDRLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "idr_latency_seconds",
				Help:      "IDR service latency in seconds",
				Buckets:   []float64{.005, .01, .025, .05, .075, .1, .15, .2},
			},
			[]string{},
		),
		IDRCircuitState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "idr_circuit_breaker_state",
				Help:      "IDR circuit breaker state (0=closed, 1=open, 2=half-open)",
			},
			[]string{},
		),
		PrivacyFiltered: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "privacy_filtered_total",
				Help:      "Total bidders filtered due to privacy",
			},
			[]string{"bidder", "reason"},
		),
		ConsentSignals: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "consent_signals_total",
				Help:      "Consent signals received",
			},
			[]string{"type", "has_consent"},
		),
		ActiveConnections: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_connections",
				Help:      "Number of active connections",
			},
		),
		RateLimitRejected: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_rejected_total",
				Help:      "Total requests rejected due to rate limiting",
			},
		),
		AuthFailures: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "auth_failures_total",
				Help:      "Total authentication failures",
			},
		),
		RevenueTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "revenue_total",
				Help:      "Total bid revenue in currency units",
			},
			[]string{"publisher", "bidder", "media_type"},
		),
		PublisherPayoutTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "publisher_payout_total",
				Help:      "Total payout to publishers in currency units",
			},
			[]string{"publisher", "bidder", "media_type"},
		),
		PlatformMarginTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "platform_margin_total",
				Help:      "Total platform margin/revenue in currency units",
			},
			[]string{"publisher", "bidder", "media_type"},
		),
		MarginPercentage: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "margin_percentage",
				Help:      "Platform margin percentage distribution",
				Buckets:   []float64{0, 1, 2, 3, 5, 7, 10, 15, 20, 25, 30, 40, 50},
			},
			[]string{"publisher"},
		),
		FloorAdjustments: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "floor_adjustments_total",
				Help:      "Number of floor price adjustments applied",
			},
			[]string{"publisher"},
		),
	}

	return m
}

func TestRecordAuction(t *testing.T) {
	m := createTestMetricsWithAll("test_auction")

	duration := 100 * time.Millisecond
	m.RecordAuction("success", "banner", duration, 5, 2)

	// Verify auction total
	count := testutil.ToFloat64(m.AuctionsTotal.WithLabelValues("success", "banner"))
	if count != 1 {
		t.Errorf("Expected 1 auction, got %v", count)
	}
}

func TestRecordBid(t *testing.T) {
	m := createTestMetricsWithAll("test_bid")

	m.RecordBid("appnexus", "banner", 2.5)
	m.RecordBid("appnexus", "banner", 3.0)

	count := testutil.ToFloat64(m.BidsReceived.WithLabelValues("appnexus", "banner"))
	if count != 2 {
		t.Errorf("Expected 2 bids, got %v", count)
	}
}

func TestRecordBidderRequest(t *testing.T) {
	m := createTestMetricsWithAll("test_bidder_req")

	tests := []struct {
		name     string
		bidder   string
		latency  time.Duration
		hasError bool
		timedOut bool
	}{
		{"success", "bidder1", 50 * time.Millisecond, false, false},
		{"error", "bidder2", 100 * time.Millisecond, true, false},
		{"timeout", "bidder3", 200 * time.Millisecond, false, true},
		{"error_and_timeout", "bidder4", 150 * time.Millisecond, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.RecordBidderRequest(tt.bidder, tt.latency, tt.hasError, tt.timedOut)

			// Verify request was counted
			count := testutil.ToFloat64(m.BidderRequests.WithLabelValues(tt.bidder))
			if count != 1 {
				t.Errorf("Expected 1 request for %s, got %v", tt.bidder, count)
			}

			// Verify error count
			if tt.hasError {
				errorCount := testutil.ToFloat64(m.BidderErrors.WithLabelValues(tt.bidder, "error"))
				if errorCount != 1 {
					t.Errorf("Expected 1 error for %s, got %v", tt.bidder, errorCount)
				}
			}

			// Verify timeout count
			if tt.timedOut {
				timeoutCount := testutil.ToFloat64(m.BidderTimeouts.WithLabelValues(tt.bidder))
				if timeoutCount != 1 {
					t.Errorf("Expected 1 timeout for %s, got %v", tt.bidder, timeoutCount)
				}
			}
		})
	}
}

func TestRecordIDRRequest(t *testing.T) {
	m := createTestMetricsWithAll("test_idr_req")

	m.RecordIDRRequest("success", 50*time.Millisecond)
	m.RecordIDRRequest("error", 100*time.Millisecond)

	successCount := testutil.ToFloat64(m.IDRRequests.WithLabelValues("success"))
	if successCount != 1 {
		t.Errorf("Expected 1 successful IDR request, got %v", successCount)
	}

	errorCount := testutil.ToFloat64(m.IDRRequests.WithLabelValues("error"))
	if errorCount != 1 {
		t.Errorf("Expected 1 error IDR request, got %v", errorCount)
	}
}

func TestSetIDRCircuitState(t *testing.T) {
	m := createTestMetricsWithAll("test_idr_circuit")

	tests := []struct {
		state    string
		expected float64
	}{
		{"closed", 0},
		{"open", 1},
		{"half-open", 2},
		{"unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			m.SetIDRCircuitState(tt.state)

			value := testutil.ToFloat64(m.IDRCircuitState.WithLabelValues())
			if value != tt.expected {
				t.Errorf("Expected state %s to have value %.0f, got %.0f", tt.state, tt.expected, value)
			}
		})
	}
}

func TestRecordPrivacyFiltered(t *testing.T) {
	m := createTestMetricsWithAll("test_privacy")

	m.RecordPrivacyFiltered("bidderA", "gdpr")
	m.RecordPrivacyFiltered("bidderA", "gdpr")
	m.RecordPrivacyFiltered("bidderB", "ccpa")

	gdprCount := testutil.ToFloat64(m.PrivacyFiltered.WithLabelValues("bidderA", "gdpr"))
	if gdprCount != 2 {
		t.Errorf("Expected 2 GDPR filters for bidderA, got %v", gdprCount)
	}

	ccpaCount := testutil.ToFloat64(m.PrivacyFiltered.WithLabelValues("bidderB", "ccpa"))
	if ccpaCount != 1 {
		t.Errorf("Expected 1 CCPA filter for bidderB, got %v", ccpaCount)
	}
}

func TestRecordConsentSignal(t *testing.T) {
	m := createTestMetricsWithAll("test_consent")

	tests := []struct {
		signalType string
		hasConsent bool
		expected   string
	}{
		{"gdpr", true, "yes"},
		{"gdpr", false, "no"},
		{"ccpa", true, "yes"},
		{"usprivacy", false, "no"},
	}

	for _, tt := range tests {
		t.Run(tt.signalType+"_"+tt.expected, func(t *testing.T) {
			m.RecordConsentSignal(tt.signalType, tt.hasConsent)

			count := testutil.ToFloat64(m.ConsentSignals.WithLabelValues(tt.signalType, tt.expected))
			if count != 1 {
				t.Errorf("Expected count of 1 for %s/%s, got %v", tt.signalType, tt.expected, count)
			}
		})
	}
}

func TestSetBidderCircuitState(t *testing.T) {
	m := createTestMetricsWithAll("test_circuit_state")

	tests := []struct {
		name          string
		state         string
		expectedValue float64
	}{
		{"closed", "closed", 0},
		{"open", "open", 1},
		{"half-open", "half-open", 2},
		{"unknown", "unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.SetBidderCircuitState("test_bidder", tt.state)

			value := testutil.ToFloat64(m.BidderCircuitState.WithLabelValues("test_bidder"))
			if value != tt.expectedValue {
				t.Errorf("Expected state value %v for %s, got %v", tt.expectedValue, tt.state, value)
			}
		})
	}
}

func TestRecordBidderCircuitRequest(t *testing.T) {
	m := createTestMetricsWithAll("test_circuit_request")

	m.RecordBidderCircuitRequest("bidderA")
	m.RecordBidderCircuitRequest("bidderA")
	m.RecordBidderCircuitRequest("bidderB")

	countA := testutil.ToFloat64(m.BidderCircuitRequests.WithLabelValues("bidderA"))
	if countA != 2 {
		t.Errorf("Expected 2 requests for bidderA, got %v", countA)
	}

	countB := testutil.ToFloat64(m.BidderCircuitRequests.WithLabelValues("bidderB"))
	if countB != 1 {
		t.Errorf("Expected 1 request for bidderB, got %v", countB)
	}
}

func TestRecordBidderCircuitFailure(t *testing.T) {
	m := createTestMetricsWithAll("test_circuit_failure")

	m.RecordBidderCircuitFailure("bidderA")
	m.RecordBidderCircuitFailure("bidderA")
	m.RecordBidderCircuitFailure("bidderA")

	count := testutil.ToFloat64(m.BidderCircuitFailures.WithLabelValues("bidderA"))
	if count != 3 {
		t.Errorf("Expected 3 failures for bidderA, got %v", count)
	}
}

func TestRecordBidderCircuitSuccess(t *testing.T) {
	m := createTestMetricsWithAll("test_circuit_success")

	m.RecordBidderCircuitSuccess("bidderA")
	m.RecordBidderCircuitSuccess("bidderA")

	count := testutil.ToFloat64(m.BidderCircuitSuccesses.WithLabelValues("bidderA"))
	if count != 2 {
		t.Errorf("Expected 2 successes for bidderA, got %v", count)
	}
}

func TestRecordBidderCircuitRejected(t *testing.T) {
	m := createTestMetricsWithAll("test_circuit_rejected")

	m.RecordBidderCircuitRejected("bidderA")
	m.RecordBidderCircuitRejected("bidderA")
	m.RecordBidderCircuitRejected("bidderB")

	countA := testutil.ToFloat64(m.BidderCircuitRejected.WithLabelValues("bidderA"))
	if countA != 2 {
		t.Errorf("Expected 2 rejected requests for bidderA, got %v", countA)
	}

	countB := testutil.ToFloat64(m.BidderCircuitRejected.WithLabelValues("bidderB"))
	if countB != 1 {
		t.Errorf("Expected 1 rejected request for bidderB, got %v", countB)
	}
}

func TestRecordBidderCircuitStateChange(t *testing.T) {
	m := createTestMetricsWithAll("test_circuit_state_change")

	m.RecordBidderCircuitStateChange("bidderA", "closed", "open")
	m.RecordBidderCircuitStateChange("bidderA", "open", "half-open")
	m.RecordBidderCircuitStateChange("bidderA", "half-open", "closed")

	closedToOpen := testutil.ToFloat64(m.BidderCircuitStateChanges.WithLabelValues("bidderA", "closed", "open"))
	if closedToOpen != 1 {
		t.Errorf("Expected 1 closed->open transition for bidderA, got %v", closedToOpen)
	}

	openToHalfOpen := testutil.ToFloat64(m.BidderCircuitStateChanges.WithLabelValues("bidderA", "open", "half-open"))
	if openToHalfOpen != 1 {
		t.Errorf("Expected 1 open->half-open transition for bidderA, got %v", openToHalfOpen)
	}

	halfOpenToClosed := testutil.ToFloat64(m.BidderCircuitStateChanges.WithLabelValues("bidderA", "half-open", "closed"))
	if halfOpenToClosed != 1 {
		t.Errorf("Expected 1 half-open->closed transition for bidderA, got %v", halfOpenToClosed)
	}
}

func TestBidderCircuitMetrics_FullScenario(t *testing.T) {
	m := createTestMetricsWithAll("test_circuit_full")

	// Scenario: Circuit breaker lifecycle for a failing bidder
	bidder := "failing_bidder"

	// Initial state is closed
	m.SetBidderCircuitState(bidder, "closed")
	if state := testutil.ToFloat64(m.BidderCircuitState.WithLabelValues(bidder)); state != 0 {
		t.Errorf("Expected initial state 0 (closed), got %v", state)
	}

	// Record 5 failures (enough to open circuit)
	for i := 0; i < 5; i++ {
		m.RecordBidderCircuitRequest(bidder)
		m.RecordBidderCircuitFailure(bidder)
	}

	// Verify requests and failures
	if requests := testutil.ToFloat64(m.BidderCircuitRequests.WithLabelValues(bidder)); requests != 5 {
		t.Errorf("Expected 5 requests, got %v", requests)
	}
	if failures := testutil.ToFloat64(m.BidderCircuitFailures.WithLabelValues(bidder)); failures != 5 {
		t.Errorf("Expected 5 failures, got %v", failures)
	}

	// Circuit opens
	m.SetBidderCircuitState(bidder, "open")
	m.RecordBidderCircuitStateChange(bidder, "closed", "open")
	if state := testutil.ToFloat64(m.BidderCircuitState.WithLabelValues(bidder)); state != 1 {
		t.Errorf("Expected state 1 (open), got %v", state)
	}

	// Reject 3 requests while open
	for i := 0; i < 3; i++ {
		m.RecordBidderCircuitRejected(bidder)
	}
	if rejected := testutil.ToFloat64(m.BidderCircuitRejected.WithLabelValues(bidder)); rejected != 3 {
		t.Errorf("Expected 3 rejected requests, got %v", rejected)
	}

	// Circuit transitions to half-open (timeout elapsed)
	m.SetBidderCircuitState(bidder, "half-open")
	m.RecordBidderCircuitStateChange(bidder, "open", "half-open")
	if state := testutil.ToFloat64(m.BidderCircuitState.WithLabelValues(bidder)); state != 2 {
		t.Errorf("Expected state 2 (half-open), got %v", state)
	}

	// Record 2 successes to close circuit
	for i := 0; i < 2; i++ {
		m.RecordBidderCircuitRequest(bidder)
		m.RecordBidderCircuitSuccess(bidder)
	}
	if successes := testutil.ToFloat64(m.BidderCircuitSuccesses.WithLabelValues(bidder)); successes != 2 {
		t.Errorf("Expected 2 successes, got %v", successes)
	}

	// Circuit closes
	m.SetBidderCircuitState(bidder, "closed")
	m.RecordBidderCircuitStateChange(bidder, "half-open", "closed")
	if state := testutil.ToFloat64(m.BidderCircuitState.WithLabelValues(bidder)); state != 0 {
		t.Errorf("Expected final state 0 (closed), got %v", state)
	}

	// Verify total state changes
	totalTransitions := 0
	for _, from := range []string{"closed", "open", "half-open"} {
		for _, to := range []string{"closed", "open", "half-open"} {
			count := testutil.ToFloat64(m.BidderCircuitStateChanges.WithLabelValues(bidder, from, to))
			totalTransitions += int(count)
		}
	}
	if totalTransitions != 3 {
		t.Errorf("Expected 3 total state transitions, got %d", totalTransitions)
	}
}
