package endpoints

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewDashboardHandler(t *testing.T) {
	handler := NewDashboardHandler()
	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestDashboardHandler_ServeHTTP(t *testing.T) {
	handler := NewDashboardHandler()

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type text/html, got %s", contentType)
	}
}

func TestNewMetricsAPIHandler(t *testing.T) {
	handler := NewMetricsAPIHandler()
	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestMetricsAPIHandler_ServeHTTP(t *testing.T) {
	// Initialize global metrics
	globalMetrics.mu.Lock()
	globalMetrics.StartTime = time.Now()
	globalMetrics.LastUpdate = time.Now()
	globalMetrics.TotalAuctions = 10
	globalMetrics.SuccessfulAuctions = 8
	globalMetrics.FailedAuctions = 2
	globalMetrics.TotalBids = 100
	globalMetrics.TotalImpressions = 5
	globalMetrics.RecentAuctions = make([]AuctionLog, 0)
	globalMetrics.BidderStats = make(map[string]int)
	globalMetrics.mu.Unlock()

	handler := NewMetricsAPIHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{0, 10, 0},
		{-5, 5, -5},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestLogAuction(t *testing.T) {
	// Reset global metrics
	globalMetrics.mu.Lock()
	globalMetrics.TotalAuctions = 0
	globalMetrics.SuccessfulAuctions = 0
	globalMetrics.FailedAuctions = 0
	globalMetrics.TotalBids = 0
	globalMetrics.TotalImpressions = 0
	globalMetrics.RecentAuctions = make([]AuctionLog, 0)
	globalMetrics.BidderStats = make(map[string]int)
	globalMetrics.mu.Unlock()

	// Log a successful auction with winning bidders
	winningBidders := []string{"appnexus", "rubicon"}

	LogAuction("req-1", 2, 5, winningBidders, 100*time.Millisecond, true, nil)

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	if globalMetrics.TotalAuctions != 1 {
		t.Errorf("expected 1 total auction, got %d", globalMetrics.TotalAuctions)
	}
	if globalMetrics.SuccessfulAuctions != 1 {
		t.Errorf("expected 1 successful auction, got %d", globalMetrics.SuccessfulAuctions)
	}
	if globalMetrics.TotalBids != 5 {
		t.Errorf("expected 5 total bids, got %d", globalMetrics.TotalBids)
	}
	if globalMetrics.TotalImpressions != 2 {
		t.Errorf("expected 2 impressions, got %d", globalMetrics.TotalImpressions)
	}
}

func TestLogAuction_Failed(t *testing.T) {
	// Reset global metrics
	globalMetrics.mu.Lock()
	globalMetrics.TotalAuctions = 0
	globalMetrics.SuccessfulAuctions = 0
	globalMetrics.FailedAuctions = 0
	globalMetrics.RecentAuctions = make([]AuctionLog, 0)
	globalMetrics.BidderStats = make(map[string]int)
	globalMetrics.mu.Unlock()

	// Log a failed auction with error
	err := errors.New("timeout")
	LogAuction("req-2", 1, 0, nil, 50*time.Millisecond, false, err)

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	if globalMetrics.TotalAuctions != 1 {
		t.Errorf("expected 1 total auction, got %d", globalMetrics.TotalAuctions)
	}
	if globalMetrics.FailedAuctions != 1 {
		t.Errorf("expected 1 failed auction, got %d", globalMetrics.FailedAuctions)
	}
	if len(globalMetrics.RecentAuctions) != 1 {
		t.Errorf("expected 1 recent auction, got %d", len(globalMetrics.RecentAuctions))
	}
	if globalMetrics.RecentAuctions[0].Error != "timeout" {
		t.Error("expected error message in auction log")
	}
}

func TestLogAuction_RecentAuctionLimit(t *testing.T) {
	// Reset global metrics
	globalMetrics.mu.Lock()
	globalMetrics.TotalAuctions = 0
	globalMetrics.RecentAuctions = make([]AuctionLog, 0)
	globalMetrics.BidderStats = make(map[string]int)
	globalMetrics.mu.Unlock()

	// Log 105 auctions (more than the 100 limit)
	for i := 0; i < 105; i++ {
		LogAuction("req", 1, 1, nil, time.Millisecond, true, nil)
	}

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	if len(globalMetrics.RecentAuctions) > 100 {
		t.Errorf("expected at most 100 recent auctions, got %d", len(globalMetrics.RecentAuctions))
	}
}

func TestGlobalMetrics_Init(t *testing.T) {
	// Verify global metrics is initialized
	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	if globalMetrics.BidderStats == nil {
		t.Error("expected BidderStats to be initialized")
	}
}

func TestAuctionLog_Fields(t *testing.T) {
	log := AuctionLog{
		Timestamp:      time.Now(),
		RequestID:      "req-1",
		ImpCount:       2,
		BidCount:       5,
		WinningBidders: []string{"bidder1"},
		Duration:       100,
		Success:        true,
		Error:          "",
	}

	if log.RequestID != "req-1" {
		t.Error("RequestID not set correctly")
	}
	if log.ImpCount != 2 {
		t.Error("ImpCount not set correctly")
	}
}

func TestDashboardMetrics_Fields(t *testing.T) {
	globalMetrics.mu.Lock()
	globalMetrics.TotalAuctions = 100
	globalMetrics.AverageDuration = 50.5
	globalMetrics.mu.Unlock()

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	if globalMetrics.TotalAuctions != 100 {
		t.Error("TotalAuctions not set correctly")
	}
}
