package stored

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractStoredRequestID(t *testing.T) {
	tests := []struct {
		name     string
		ext      json.RawMessage
		expected string
	}{
		{
			name:     "nil ext",
			ext:      nil,
			expected: "",
		},
		{
			name:     "empty ext",
			ext:      json.RawMessage(`{}`),
			expected: "",
		},
		{
			name:     "no prebid",
			ext:      json.RawMessage(`{"other": "data"}`),
			expected: "",
		},
		{
			name:     "no storedrequest",
			ext:      json.RawMessage(`{"prebid": {"other": "data"}}`),
			expected: "",
		},
		{
			name:     "with stored request id",
			ext:      json.RawMessage(`{"prebid": {"storedrequest": {"id": "stored-123"}}}`),
			expected: "stored-123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, _ := ExtractStoredRequestID(tc.ext)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestExtractStoredImpID(t *testing.T) {
	tests := []struct {
		name     string
		ext      json.RawMessage
		expected string
	}{
		{
			name:     "with stored imp id",
			ext:      json.RawMessage(`{"prebid": {"storedrequest": {"id": "imp-456"}}}`),
			expected: "imp-456",
		},
		{
			name:     "no stored imp id",
			ext:      json.RawMessage(`{"bidder": {"param": "value"}}`),
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, _ := ExtractStoredImpID(tc.ext)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name     string
		dst      map[string]interface{}
		src      map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "simple overwrite",
			dst:  map[string]interface{}{"a": 1, "b": 2},
			src:  map[string]interface{}{"b": 3},
			expected: map[string]interface{}{
				"a": 1,
				"b": 3,
			},
		},
		{
			name: "nested merge",
			dst: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": 1,
					"b": 2,
				},
			},
			src: map[string]interface{}{
				"outer": map[string]interface{}{
					"b": 3,
					"c": 4,
				},
			},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": 1,
					"b": 3,
					"c": 4,
				},
			},
		},
		{
			name: "src adds new keys",
			dst:  map[string]interface{}{"a": 1},
			src:  map[string]interface{}{"b": 2},
			expected: map[string]interface{}{
				"a": 1,
				"b": 2,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := deepMerge(tc.dst, tc.src)

			// Compare as JSON for easier debugging
			resultJSON, _ := json.Marshal(result)
			expectedJSON, _ := json.Marshal(tc.expected)

			if string(resultJSON) != string(expectedJSON) {
				t.Errorf("expected %s, got %s", string(expectedJSON), string(resultJSON))
			}
		})
	}
}

// mockFetcher implements Fetcher for testing
type mockFetcher struct {
	requests    map[string]json.RawMessage
	impressions map[string]json.RawMessage
	responses   map[string]json.RawMessage
	accounts    map[string]json.RawMessage
}

func newMockFetcher() *mockFetcher {
	return &mockFetcher{
		requests:    make(map[string]json.RawMessage),
		impressions: make(map[string]json.RawMessage),
		responses:   make(map[string]json.RawMessage),
		accounts:    make(map[string]json.RawMessage),
	}
}

func (m *mockFetcher) FetchRequests(ctx context.Context, requestIDs []string) (map[string]json.RawMessage, []error) {
	result := make(map[string]json.RawMessage)
	var errs []error
	for _, id := range requestIDs {
		if data, ok := m.requests[id]; ok {
			result[id] = data
		} else {
			errs = append(errs, ErrNotFound)
		}
	}
	return result, errs
}

func (m *mockFetcher) FetchImpressions(ctx context.Context, impIDs []string) (map[string]json.RawMessage, []error) {
	result := make(map[string]json.RawMessage)
	var errs []error
	for _, id := range impIDs {
		if data, ok := m.impressions[id]; ok {
			result[id] = data
		} else {
			errs = append(errs, ErrNotFound)
		}
	}
	return result, errs
}

func (m *mockFetcher) FetchResponses(ctx context.Context, respIDs []string) (map[string]json.RawMessage, []error) {
	result := make(map[string]json.RawMessage)
	var errs []error
	for _, id := range respIDs {
		if data, ok := m.responses[id]; ok {
			result[id] = data
		} else {
			errs = append(errs, ErrNotFound)
		}
	}
	return result, errs
}

func (m *mockFetcher) FetchAccount(ctx context.Context, accountID string) (json.RawMessage, error) {
	if data, ok := m.accounts[accountID]; ok {
		return data, nil
	}
	return nil, ErrNotFound
}

func (m *mockFetcher) Close() error {
	return nil
}

func TestCache_FetchRequests(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{"id": "req-1", "site": {"domain": "example.com"}}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Minute})

	ctx := context.Background()

	// First fetch - should hit backend
	result, errs := cache.FetchRequests(ctx, []string{"req-1"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if _, ok := result["req-1"]; !ok {
		t.Error("expected to find req-1")
	}

	// Second fetch - should hit cache
	result, errs = cache.FetchRequests(ctx, []string{"req-1"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors on cached fetch: %v", errs)
	}
	if _, ok := result["req-1"]; !ok {
		t.Error("expected to find req-1 from cache")
	}

	// Fetch non-existent
	result, errs = cache.FetchRequests(ctx, []string{"req-999"})
	if len(errs) == 0 {
		t.Error("expected error for non-existent request")
	}
}

func TestCache_Invalidate(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{"id": "req-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})

	ctx := context.Background()

	// Populate cache
	cache.FetchRequests(ctx, []string{"req-1"})

	// Verify it's cached
	stats := cache.Stats()
	if stats.RequestCount != 1 {
		t.Errorf("expected 1 cached request, got %d", stats.RequestCount)
	}

	// Invalidate
	cache.Invalidate(DataTypeRequest, []string{"req-1"})

	// Verify it's removed
	stats = cache.Stats()
	if stats.RequestCount != 0 {
		t.Errorf("expected 0 cached requests after invalidate, got %d", stats.RequestCount)
	}
}

func TestCache_InvalidateAll(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{"id": "req-1"}`)
	mock.impressions["imp-1"] = json.RawMessage(`{"id": "imp-1"}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})

	ctx := context.Background()

	// Populate cache
	cache.FetchRequests(ctx, []string{"req-1"})
	cache.FetchImpressions(ctx, []string{"imp-1"})

	// Verify populated
	stats := cache.Stats()
	if stats.RequestCount != 1 || stats.ImpressionCount != 1 {
		t.Error("expected cache to be populated")
	}

	// Invalidate all
	cache.InvalidateAll()

	// Verify cleared
	stats = cache.Stats()
	if stats.RequestCount != 0 || stats.ImpressionCount != 0 {
		t.Error("expected cache to be cleared")
	}
}

func TestMerger_NoStoredID(t *testing.T) {
	mock := newMockFetcher()
	merger := NewMerger(mock)

	incoming := json.RawMessage(`{"id": "req-1", "site": {"domain": "example.com"}}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StoredRequestID != "" {
		t.Errorf("expected no stored request ID, got %q", result.StoredRequestID)
	}

	// Result should be same as incoming
	if string(result.MergedData) != string(incoming) {
		t.Errorf("expected merged data to equal incoming")
	}
}

func TestMerger_WithStoredRequest(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["stored-123"] = json.RawMessage(`{
		"site": {
			"domain": "stored-domain.com",
			"publisher": {"id": "pub-1"}
		},
		"user": {"id": "user-1"}
	}`)

	merger := NewMerger(mock)

	// Incoming request references stored request and overrides domain
	incoming := json.RawMessage(`{
		"id": "req-1",
		"site": {"domain": "incoming-domain.com"},
		"ext": {"prebid": {"storedrequest": {"id": "stored-123"}}}
	}`)

	result, err := merger.MergeRequest(context.Background(), incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StoredRequestID != "stored-123" {
		t.Errorf("expected stored request ID 'stored-123', got %q", result.StoredRequestID)
	}

	// Parse merged result
	var merged map[string]interface{}
	if err := json.Unmarshal(result.MergedData, &merged); err != nil {
		t.Fatalf("failed to parse merged result: %v", err)
	}

	// Check that incoming domain overrides stored domain
	site := merged["site"].(map[string]interface{})
	if site["domain"] != "incoming-domain.com" {
		t.Errorf("expected incoming domain to override stored, got %v", site["domain"])
	}

	// Check that stored publisher is preserved
	publisher := site["publisher"].(map[string]interface{})
	if publisher["id"] != "pub-1" {
		t.Errorf("expected stored publisher to be preserved, got %v", publisher["id"])
	}

	// Check that stored user is preserved
	user := merged["user"].(map[string]interface{})
	if user["id"] != "user-1" {
		t.Errorf("expected stored user to be preserved, got %v", user["id"])
	}
}

func TestFilesystemFetcher(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	ctx := context.Background()

	// Save a request
	reqData := json.RawMessage(`{"id": "test-req", "site": {"domain": "example.com"}}`)
	if err := fetcher.SaveRequest("test-req", reqData); err != nil {
		t.Fatalf("failed to save request: %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(tmpDir, "requests", "test-req.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("expected request file to exist")
	}

	// Fetch the request
	result, errs := fetcher.FetchRequests(ctx, []string{"test-req"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if _, ok := result["test-req"]; !ok {
		t.Error("expected to find test-req")
	}

	// Fetch non-existent
	result, errs = fetcher.FetchRequests(ctx, []string{"non-existent"})
	if len(errs) == 0 {
		t.Error("expected error for non-existent request")
	}

	// List requests
	ids, err := fetcher.ListRequests()
	if err != nil {
		t.Fatalf("failed to list requests: %v", err)
	}
	if len(ids) != 1 || ids[0] != "test-req" {
		t.Errorf("expected [test-req], got %v", ids)
	}

	// Delete request
	if err := fetcher.Delete(DataTypeRequest, "test-req"); err != nil {
		t.Fatalf("failed to delete request: %v", err)
	}

	// Verify deleted
	result, errs = fetcher.FetchRequests(ctx, []string{"test-req"})
	if len(errs) == 0 {
		t.Error("expected error after delete")
	}
}

func TestFilesystemFetcher_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stored-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fetcher, err := NewFilesystemFetcher(FilesystemConfig{BaseDir: tmpDir})
	if err != nil {
		t.Fatalf("failed to create filesystem fetcher: %v", err)
	}

	// Try to save invalid JSON
	err = fetcher.SaveRequest("invalid", json.RawMessage(`not valid json`))
	if err != ErrInvalidJSON {
		t.Errorf("expected ErrInvalidJSON, got %v", err)
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	if config.TTL != 5*time.Minute {
		t.Errorf("expected TTL of 5 minutes, got %v", config.TTL)
	}
	if config.MaxEntries != 10000 {
		t.Errorf("expected MaxEntries of 10000, got %d", config.MaxEntries)
	}
}

func TestDefaultPostgresConfig(t *testing.T) {
	config := DefaultPostgresConfig()

	if config.RequestsTable != "stored_requests" {
		t.Errorf("expected RequestsTable 'stored_requests', got %q", config.RequestsTable)
	}
	if config.QueryTimeout != 5*time.Second {
		t.Errorf("expected QueryTimeout of 5s, got %v", config.QueryTimeout)
	}
}

func TestCacheStats(t *testing.T) {
	mock := newMockFetcher()
	mock.requests["req-1"] = json.RawMessage(`{}`)
	mock.requests["req-2"] = json.RawMessage(`{}`)
	mock.impressions["imp-1"] = json.RawMessage(`{}`)

	cache := NewCache(mock, CacheConfig{TTL: 1 * time.Hour})
	ctx := context.Background()

	// Initial stats should be zero
	stats := cache.Stats()
	if stats.RequestCount != 0 || stats.ImpressionCount != 0 {
		t.Error("expected empty cache initially")
	}

	// Populate cache
	cache.FetchRequests(ctx, []string{"req-1", "req-2"})
	cache.FetchImpressions(ctx, []string{"imp-1"})

	// Check stats
	stats = cache.Stats()
	if stats.RequestCount != 2 {
		t.Errorf("expected 2 requests, got %d", stats.RequestCount)
	}
	if stats.ImpressionCount != 1 {
		t.Errorf("expected 1 impression, got %d", stats.ImpressionCount)
	}
}

func TestDataType_Constants(t *testing.T) {
	// Verify data type constants
	if DataTypeRequest != "request" {
		t.Error("unexpected DataTypeRequest value")
	}
	if DataTypeImpression != "impression" {
		t.Error("unexpected DataTypeImpression value")
	}
	if DataTypeResponse != "response" {
		t.Error("unexpected DataTypeResponse value")
	}
	if DataTypeAccount != "account" {
		t.Error("unexpected DataTypeAccount value")
	}
}
