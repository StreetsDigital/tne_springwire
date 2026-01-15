package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

func TestEngine_LogEvent(t *testing.T) {
	adapter := NewMemoryAdapter(100)
	engine := NewEngine(DefaultConfig())
	engine.AddAdapter(adapter)
	defer engine.Close()

	event := &AuctionEvent{
		Type:      EventAuctionStart,
		RequestID: "test-123",
		Timestamp: time.Now(),
	}

	engine.LogEvent(event)

	// Wait for worker to process
	time.Sleep(50 * time.Millisecond)

	if adapter.Count() != 1 {
		t.Errorf("expected 1 event, got %d", adapter.Count())
	}
}

func TestEngine_LogAuctionStart(t *testing.T) {
	adapter := NewMemoryAdapter(100)
	engine := NewEngine(DefaultConfig())
	engine.AddAdapter(adapter)
	defer engine.Close()

	req := &openrtb.BidRequest{
		ID: "auction-123",
		Site: &openrtb.Site{
			Domain:    "example.com",
			Publisher: &openrtb.Publisher{ID: "pub-456"},
		},
	}

	engine.LogAuctionStart(req)

	time.Sleep(50 * time.Millisecond)

	events := adapter.GetEventsByType(EventAuctionStart)
	if len(events) != 1 {
		t.Fatalf("expected 1 auction start event, got %d", len(events))
	}

	event := events[0]
	if event.RequestID != "auction-123" {
		t.Errorf("expected request ID 'auction-123', got '%s'", event.RequestID)
	}
	if event.Domain != "example.com" {
		t.Errorf("expected domain 'example.com', got '%s'", event.Domain)
	}
	if event.PublisherID != "pub-456" {
		t.Errorf("expected publisher ID 'pub-456', got '%s'", event.PublisherID)
	}
}

func TestEngine_LogBidResponse(t *testing.T) {
	adapter := NewMemoryAdapter(100)
	engine := NewEngine(DefaultConfig())
	engine.AddAdapter(adapter)
	defer engine.Close()

	bids := []openrtb.Bid{
		{ID: "bid-1", ImpID: "imp-1", Price: 1.50},
		{ID: "bid-2", ImpID: "imp-2", Price: 2.00},
	}

	engine.LogBidResponse("req-123", "appnexus", bids, 50*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	events := adapter.GetEventsByType(EventBidResponse)
	if len(events) != 2 {
		t.Fatalf("expected 2 bid response events, got %d", len(events))
	}

	// Check first bid
	if events[0].BidID != "bid-1" && events[1].BidID != "bid-1" {
		t.Error("expected bid-1 in events")
	}
}

func TestEngine_Disabled(t *testing.T) {
	adapter := NewMemoryAdapter(100)
	config := DefaultConfig()
	config.Enabled = false

	engine := NewEngine(config)
	engine.AddAdapter(adapter)
	defer engine.Close()

	engine.LogEvent(&AuctionEvent{Type: EventAuctionStart, RequestID: "test"})

	time.Sleep(50 * time.Millisecond)

	if adapter.Count() != 0 {
		t.Errorf("expected 0 events when disabled, got %d", adapter.Count())
	}
}

func TestEngine_SampleRate(t *testing.T) {
	adapter := NewMemoryAdapter(1000)
	config := DefaultConfig()
	config.SampleRate = 0.5 // 50% sampling

	engine := NewEngine(config)
	engine.AddAdapter(adapter)
	defer engine.Close()

	// Log many events with unique request IDs
	for i := 0; i < 100; i++ {
		engine.LogEvent(&AuctionEvent{
			Type:      EventAuctionStart,
			RequestID: "request-" + itoa(i) + "-unique-id",
		})
	}

	time.Sleep(100 * time.Millisecond)

	// With 50% sampling, should have roughly 30-70 events (wider range for randomness)
	count := adapter.Count()
	if count < 20 || count > 80 {
		t.Errorf("expected ~50 events with 50%% sampling, got %d", count)
	}
}

// itoa converts int to string for test
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestMemoryAdapter(t *testing.T) {
	adapter := NewMemoryAdapter(5) // Small max size

	// Add more events than max
	for i := 0; i < 10; i++ {
		adapter.LogAuctionEvent(context.Background(), &AuctionEvent{
			Type:      EventAuctionStart,
			RequestID: string(rune('0' + i)),
		})
	}

	if adapter.Count() != 5 {
		t.Errorf("expected 5 events (max size), got %d", adapter.Count())
	}

	// Clear
	adapter.Clear()
	if adapter.Count() != 0 {
		t.Errorf("expected 0 events after clear, got %d", adapter.Count())
	}
}

func TestMemoryAdapter_GetEventsByType(t *testing.T) {
	adapter := NewMemoryAdapter(100)

	adapter.LogAuctionEvent(context.Background(), &AuctionEvent{Type: EventAuctionStart})
	adapter.LogAuctionEvent(context.Background(), &AuctionEvent{Type: EventAuctionEnd})
	adapter.LogAuctionEvent(context.Background(), &AuctionEvent{Type: EventAuctionStart})
	adapter.LogAuctionEvent(context.Background(), &AuctionEvent{Type: EventBidResponse})

	starts := adapter.GetEventsByType(EventAuctionStart)
	if len(starts) != 2 {
		t.Errorf("expected 2 auction start events, got %d", len(starts))
	}

	responses := adapter.GetEventsByType(EventBidResponse)
	if len(responses) != 1 {
		t.Errorf("expected 1 bid response event, got %d", len(responses))
	}
}

func TestFileAdapter(t *testing.T) {
	// Create temp file
	tmpfile, err := os.CreateTemp("", "analytics-test-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	adapter, err := NewFileAdapter(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()

	if adapter.Name() != "file" {
		t.Errorf("expected name 'file', got '%s'", adapter.Name())
	}

	// Log an event
	event := &AuctionEvent{
		Type:      EventAuctionStart,
		RequestID: "test-file-123",
		Timestamp: time.Now(),
	}

	err = adapter.LogAuctionEvent(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}

	// Close to flush
	adapter.Close()

	// Read file and verify
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	var logged AuctionEvent
	if err := json.Unmarshal(data, &logged); err != nil {
		t.Fatal(err)
	}

	if logged.RequestID != "test-file-123" {
		t.Errorf("expected request ID 'test-file-123', got '%s'", logged.RequestID)
	}
}

func TestStdoutAdapter(t *testing.T) {
	adapter := NewStdoutAdapter(false)

	if adapter.Name() != "stdout" {
		t.Errorf("expected name 'stdout', got '%s'", adapter.Name())
	}

	// Just verify it doesn't panic
	err := adapter.Close()
	if err != nil {
		t.Error("Close should not return error")
	}
}

func TestHTTPAdapter(t *testing.T) {
	var mu sync.Mutex
	receivedEvents := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedEvents++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewHTTPAdapter(&HTTPAdapterConfig{
		Endpoint:      server.URL,
		APIKey:        "test-key",
		BatchSize:     2,
		FlushInterval: 100 * time.Millisecond,
	})

	if adapter.Name() != "http" {
		t.Errorf("expected name 'http', got '%s'", adapter.Name())
	}

	// Log events to trigger batch
	adapter.LogAuctionEvent(context.Background(), &AuctionEvent{Type: EventAuctionStart})
	adapter.LogAuctionEvent(context.Background(), &AuctionEvent{Type: EventAuctionEnd})

	// Wait for batch to be sent
	time.Sleep(200 * time.Millisecond)

	// Close adapter before checking result
	adapter.Close()

	mu.Lock()
	count := receivedEvents
	mu.Unlock()

	if count == 0 {
		t.Error("expected HTTP endpoint to receive events")
	}
}

func TestEngine_RemoveAdapter(t *testing.T) {
	adapter1 := NewMemoryAdapter(100)
	adapter2 := NewMemoryAdapter(100)

	engine := NewEngine(DefaultConfig())
	engine.AddAdapter(adapter1)
	engine.AddAdapter(adapter2)
	defer engine.Close()

	// Remove first adapter
	engine.RemoveAdapter("memory")

	// Log event - should only go to remaining adapter
	engine.LogEvent(&AuctionEvent{Type: EventAuctionStart, RequestID: "test"})

	time.Sleep(50 * time.Millisecond)

	// Only one adapter should remain and receive the event
	total := adapter1.Count() + adapter2.Count()
	if total != 1 {
		t.Errorf("expected 1 total event (one adapter removed), got %d", total)
	}
}

func TestShouldSample(t *testing.T) {
	// 100% sample rate should always return true
	if !shouldSample("any-id", 1.0) {
		t.Error("expected true for 100% sample rate")
	}

	// 0% sample rate should always return false
	if shouldSample("any-id", 0.0) {
		t.Error("expected false for 0% sample rate")
	}

	// Deterministic - same ID should give same result
	result1 := shouldSample("test-id-123", 0.5)
	result2 := shouldSample("test-id-123", 0.5)
	if result1 != result2 {
		t.Error("expected same result for same ID")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("expected enabled by default")
	}

	if config.BufferSize != 10000 {
		t.Errorf("expected buffer size 10000, got %d", config.BufferSize)
	}

	if config.Workers != 4 {
		t.Errorf("expected 4 workers, got %d", config.Workers)
	}

	if config.SampleRate != 1.0 {
		t.Errorf("expected sample rate 1.0, got %f", config.SampleRate)
	}
}
