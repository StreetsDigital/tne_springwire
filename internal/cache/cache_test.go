package cache

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_StoreBids(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req CacheRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}

		if len(req.Puts) != 2 {
			t.Errorf("expected 2 puts, got %d", len(req.Puts))
		}

		resp := CacheResponse{
			Responses: []CacheResponseItem{
				{UUID: "uuid-1"},
				{UUID: "uuid-2"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.Endpoint = server.URL
	client := NewClient(config)

	bids := []string{`{"id":"bid1"}`, `{"id":"bid2"}`}
	results, err := client.StoreBids(context.Background(), bids)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].UUID != "uuid-1" {
		t.Errorf("expected uuid-1, got %s", results[0].UUID)
	}

	if results[1].UUID != "uuid-2" {
		t.Errorf("expected uuid-2, got %s", results[1].UUID)
	}
}

func TestClient_StoreVAST(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req CacheRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Puts) != 1 {
			t.Errorf("expected 1 put for VAST, got %d", len(req.Puts))
		}

		if req.Puts[0].Type != "xml" {
			t.Errorf("expected type 'xml', got '%s'", req.Puts[0].Type)
		}

		resp := CacheResponse{
			Responses: []CacheResponseItem{{UUID: "vast-uuid"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.Endpoint = server.URL
	client := NewClient(config)

	result, err := client.StoreVAST(context.Background(), "<VAST>...</VAST>", 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if result.UUID != "vast-uuid" {
		t.Errorf("expected vast-uuid, got %s", result.UUID)
	}
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uuid := r.URL.Query().Get("uuid")
		if uuid == "test-uuid" {
			w.Write([]byte(`{"id":"cached-bid"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.Endpoint = server.URL
	config.UseLocalCache = false
	client := NewClient(config)

	value, err := client.Get(context.Background(), "test-uuid")
	if err != nil {
		t.Fatal(err)
	}

	if value != `{"id":"cached-bid"}` {
		t.Errorf("unexpected value: %s", value)
	}
}

func TestClient_LocalCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			resp := CacheResponse{
				Responses: []CacheResponseItem{{UUID: "local-test"}},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	config := DefaultConfig()
	config.Endpoint = server.URL
	config.UseLocalCache = true
	client := NewClient(config)

	// Store a bid
	_, err := client.StoreBids(context.Background(), []string{`{"test":"value"}`})
	if err != nil {
		t.Fatal(err)
	}

	// Should be in local cache
	value, err := client.Get(context.Background(), "local-test")
	if err != nil {
		t.Fatal(err)
	}

	if value != `{"test":"value"}` {
		t.Errorf("expected value from local cache, got: %s", value)
	}

	// Check stats
	size, expired := client.LocalCacheStats()
	if size != 1 {
		t.Errorf("expected 1 item in cache, got %d", size)
	}
	if expired != 0 {
		t.Errorf("expected 0 expired, got %d", expired)
	}
}

func TestClient_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	client := NewClient(config)

	results, err := client.StoreBids(context.Background(), []string{`{"test":"value"}`})
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Error("expected nil results when disabled")
	}
}

func TestClient_NoEndpoint(t *testing.T) {
	config := DefaultConfig()
	config.Endpoint = ""
	client := NewClient(config)

	results, err := client.StoreBids(context.Background(), []string{`{"test":"value"}`})
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Error("expected nil results with no endpoint")
	}
}

func TestClient_ClearLocalCache(t *testing.T) {
	config := DefaultConfig()
	config.UseLocalCache = true
	client := NewClient(config)

	// Add something to local cache manually
	client.mu.Lock()
	client.localCache["test"] = &cacheEntry{
		value:     "test",
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	client.mu.Unlock()

	size, _ := client.LocalCacheStats()
	if size != 1 {
		t.Errorf("expected 1 item before clear, got %d", size)
	}

	client.ClearLocalCache()

	size, _ = client.LocalCacheStats()
	if size != 0 {
		t.Errorf("expected 0 items after clear, got %d", size)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("expected enabled by default")
	}

	if config.Timeout != 100*time.Millisecond {
		t.Errorf("expected 100ms timeout, got %v", config.Timeout)
	}

	if config.DefaultTTL != 5*time.Minute {
		t.Errorf("expected 5 minute TTL, got %v", config.DefaultTTL)
	}

	if !config.UseLocalCache {
		t.Error("expected local cache enabled by default")
	}
}
