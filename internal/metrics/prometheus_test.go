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
