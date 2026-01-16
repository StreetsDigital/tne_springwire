package endpoints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestLogAuction_EdgeCases tests LogAuction with various edge cases
func TestLogAuction_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		requestID      string
		impCount       int
		bidCount       int
		winningBidders []string
		duration       time.Duration
		success        bool
		err            error
	}{
		{
			name:           "Empty winning bidders",
			requestID:      "req-empty-bidders",
			impCount:       3,
			bidCount:       0,
			winningBidders: []string{},
			duration:       100 * time.Millisecond,
			success:        true,
			err:            nil,
		},
		{
			name:           "Zero duration",
			requestID:      "req-zero-duration",
			impCount:       2,
			bidCount:       5,
			winningBidders: []string{"appnexus", "rubicon"},
			duration:       0,
			success:        true,
			err:            nil,
		},
		{
			name:           "Failed auction with error",
			requestID:      "req-failed",
			impCount:       1,
			bidCount:       0,
			winningBidders: []string{},
			duration:       50 * time.Millisecond,
			success:        false,
			err:            http.ErrServerClosed,
		},
		{
			name:           "Large number of bidders",
			requestID:      "req-many-bidders",
			impCount:       10,
			bidCount:       20,
			winningBidders: []string{"bidder1", "bidder2", "bidder3", "bidder4", "bidder5"},
			duration:       200 * time.Millisecond,
			success:        true,
			err:            nil,
		},
		{
			name:           "Nil winning bidders",
			requestID:      "req-nil-bidders",
			impCount:       1,
			bidCount:       0,
			winningBidders: nil,
			duration:       10 * time.Millisecond,
			success:        true,
			err:            nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global metrics before each test
			globalMetrics = &DashboardMetrics{
				BidderStats:    make(map[string]int),
				RecentAuctions: make([]AuctionLog, 0, 100),
				StartTime:      time.Now(),
				LastUpdate:     time.Now(),
			}

			LogAuction(tt.requestID, tt.impCount, tt.bidCount, tt.winningBidders, tt.duration, tt.success, tt.err)

			globalMetrics.mu.RLock()
			defer globalMetrics.mu.RUnlock()

			// Verify auction was logged
			if len(globalMetrics.RecentAuctions) != 1 {
				t.Errorf("Expected 1 auction logged, got %d", len(globalMetrics.RecentAuctions))
			}

			if len(globalMetrics.RecentAuctions) > 0 {
				auction := globalMetrics.RecentAuctions[0]
				if auction.RequestID != tt.requestID {
					t.Errorf("Expected request ID %s, got %s", tt.requestID, auction.RequestID)
				}
				if auction.ImpCount != tt.impCount {
					t.Errorf("Expected imp count %d, got %d", tt.impCount, auction.ImpCount)
				}
				if auction.BidCount != tt.bidCount {
					t.Errorf("Expected bid count %d, got %d", tt.bidCount, auction.BidCount)
				}
				if auction.Success != tt.success {
					t.Errorf("Expected success %v, got %v", tt.success, auction.Success)
				}
			}

			// Verify total auctions incremented
			if globalMetrics.TotalAuctions != 1 {
				t.Errorf("Expected total auctions 1, got %d", globalMetrics.TotalAuctions)
			}

			// Verify successful/failed count
			if tt.success && globalMetrics.SuccessfulAuctions != 1 {
				t.Errorf("Expected successful auctions 1, got %d", globalMetrics.SuccessfulAuctions)
			}
			if !tt.success && globalMetrics.FailedAuctions != 1 {
				t.Errorf("Expected failed auctions 1, got %d", globalMetrics.FailedAuctions)
			}
		})
	}
}

// TestLogAuction_CircularBuffer tests that auction log maintains 100-item limit
func TestLogAuction_CircularBuffer(t *testing.T) {
	// Reset global metrics
	globalMetrics = &DashboardMetrics{
		BidderStats:    make(map[string]int),
		RecentAuctions: make([]AuctionLog, 0, 100),
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
	}

	// Log 150 auctions to test circular buffer
	for i := 0; i < 150; i++ {
		LogAuction(
			"req-"+string(rune(i)),
			1,
			1,
			[]string{"appnexus"},
			100*time.Millisecond,
			true,
			nil,
		)
	}

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	// Should only keep last 100 auctions
	if len(globalMetrics.RecentAuctions) != 100 {
		t.Errorf("Expected 100 recent auctions, got %d", len(globalMetrics.RecentAuctions))
	}

	// Total auctions should be 150
	if globalMetrics.TotalAuctions != 150 {
		t.Errorf("Expected total auctions 150, got %d", globalMetrics.TotalAuctions)
	}
}

// TestLogAuction_BidderStats tests bidder statistics accumulation
func TestLogAuction_BidderStats(t *testing.T) {
	// Reset global metrics
	globalMetrics = &DashboardMetrics{
		BidderStats:    make(map[string]int),
		RecentAuctions: make([]AuctionLog, 0, 100),
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
	}

	// Log auctions with different winning bidders
	LogAuction("req-1", 1, 3, []string{"appnexus"}, 100*time.Millisecond, true, nil)
	LogAuction("req-2", 1, 3, []string{"rubicon"}, 100*time.Millisecond, true, nil)
	LogAuction("req-3", 1, 3, []string{"appnexus", "pubmatic"}, 100*time.Millisecond, true, nil)

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	// Verify bidder stats
	expectedStats := map[string]int{
		"appnexus": 2,
		"rubicon":  1,
		"pubmatic": 1,
	}

	for bidder, expectedCount := range expectedStats {
		if count, ok := globalMetrics.BidderStats[bidder]; !ok {
			t.Errorf("Expected bidder %s in stats", bidder)
		} else if count != expectedCount {
			t.Errorf("Expected bidder %s count %d, got %d", bidder, expectedCount, count)
		}
	}
}

// TestLogAuction_Concurrent tests concurrent logging
func TestLogAuction_Concurrent(t *testing.T) {
	// Reset global metrics
	globalMetrics = &DashboardMetrics{
		BidderStats:    make(map[string]int),
		RecentAuctions: make([]AuctionLog, 0, 100),
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			LogAuction(
				"req-concurrent",
				1,
				2,
				[]string{"appnexus"},
				100*time.Millisecond,
				true,
				nil,
			)
		}(i)
	}

	wg.Wait()

	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()

	// Verify total auctions
	if globalMetrics.TotalAuctions != int64(numGoroutines) {
		t.Errorf("Expected total auctions %d, got %d", numGoroutines, globalMetrics.TotalAuctions)
	}

	// Verify successful auctions
	if globalMetrics.SuccessfulAuctions != int64(numGoroutines) {
		t.Errorf("Expected successful auctions %d, got %d", numGoroutines, globalMetrics.SuccessfulAuctions)
	}
}

// TestNewDashboardHandler tests dashboard handler creation
func TestNewDashboardHandler(t *testing.T) {
	handler := NewDashboardHandler()
	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
}

// TestDashboardHandler_ServeHTTP tests dashboard HTML rendering
func TestDashboardHandler_ServeHTTP(t *testing.T) {
	handler := NewDashboardHandler()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected content type text/html, got %s", contentType)
	}

	// Verify response contains expected data
	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty response body")
	}

	// Check for key dashboard HTML elements (dashboard uses JavaScript to load data)
	expectedStrings := []string{
		"<!DOCTYPE html>",
		"Nexus Exchange",
		"Dashboard",
	}

	for _, str := range expectedStrings {
		if !contains(body, str) {
			t.Errorf("Expected response to contain '%s'", str)
		}
	}
}

// TestDashboardHandler_ServeHTTP_EmptyMetrics tests dashboard with no auctions
func TestDashboardHandler_ServeHTTP_EmptyMetrics(t *testing.T) {
	// Reset metrics to empty state
	globalMetrics = &DashboardMetrics{
		BidderStats:    make(map[string]int),
		RecentAuctions: make([]AuctionLog, 0, 100),
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
	}

	handler := NewDashboardHandler()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should still return 200 with empty state
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !contains(body, "Total Auctions") {
		t.Error("Expected response to contain dashboard header")
	}
}

// TestNewMetricsAPIHandler tests metrics API handler creation
func TestNewMetricsAPIHandler(t *testing.T) {
	handler := NewMetricsAPIHandler()
	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
}

// TestMetricsAPIHandler_ServeHTTP tests metrics JSON API
func TestMetricsAPIHandler_ServeHTTP(t *testing.T) {
	// Store original metrics to restore after test
	originalMetrics := globalMetrics

	// Create fresh metrics for this test
	globalMetrics = &DashboardMetrics{
		BidderStats:    make(map[string]int),
		RecentAuctions: make([]AuctionLog, 0, 100),
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
	}

	// Restore original metrics after test
	defer func() {
		globalMetrics = originalMetrics
	}()

	// Log test data
	LogAuction("req-test-1", 2, 5, []string{"appnexus", "rubicon"}, 150*time.Millisecond, true, nil)
	LogAuction("req-test-2", 1, 3, []string{"appnexus"}, 100*time.Millisecond, true, nil)

	handler := NewMetricsAPIHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected content type application/json, got %s", contentType)
	}

	// Parse JSON response - note: the response is a map, not DashboardMetrics directly
	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	if err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Verify metrics exist in response
	if totalAuctions, ok := response["total_auctions"].(float64); !ok {
		t.Error("Expected total_auctions in response")
	} else if int64(totalAuctions) != 2 {
		t.Errorf("Expected total auctions 2, got %d", int64(totalAuctions))
	}

	if successfulAuctions, ok := response["successful_auctions"].(float64); !ok {
		t.Error("Expected successful_auctions in response")
	} else if int64(successfulAuctions) != 2 {
		t.Errorf("Expected successful auctions 2, got %d", int64(successfulAuctions))
	}
}

// TestMetricsAPIHandler_ServeHTTP_EmptyMetrics tests API with no auctions
func TestMetricsAPIHandler_ServeHTTP_EmptyMetrics(t *testing.T) {
	// Reset metrics to empty state
	globalMetrics = &DashboardMetrics{
		BidderStats:    make(map[string]int),
		RecentAuctions: make([]AuctionLog, 0, 100),
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
	}

	handler := NewMetricsAPIHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Parse JSON response
	var metrics DashboardMetrics
	err := json.NewDecoder(w.Body).Decode(&metrics)
	if err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Verify empty metrics
	if metrics.TotalAuctions != 0 {
		t.Errorf("Expected total auctions 0, got %d", metrics.TotalAuctions)
	}

	if metrics.SuccessfulAuctions != 0 {
		t.Errorf("Expected successful auctions 0, got %d", metrics.SuccessfulAuctions)
	}

	if len(metrics.BidderStats) != 0 {
		t.Errorf("Expected empty bidder stats, got %d entries", len(metrics.BidderStats))
	}

	if len(metrics.RecentAuctions) != 0 {
		t.Errorf("Expected 0 recent auctions, got %d", len(metrics.RecentAuctions))
	}
}

// TestMetricsAPIHandler_ServeHTTP_POST tests non-GET method
func TestMetricsAPIHandler_ServeHTTP_POST(t *testing.T) {
	handler := NewMetricsAPIHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/metrics", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should still return 200 (handler doesn't restrict methods)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOfSubstring(s, substr) >= 0)
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
