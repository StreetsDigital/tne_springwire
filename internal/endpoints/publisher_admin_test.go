package endpoints

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/thenexusengine/tne_springwire/pkg/redis"
)

// setupTestRedisForPublisher creates a test Redis client with miniredis
func setupTestRedisForPublisher(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	// Create miniredis server
	mr := miniredis.RunT(t)

	// Create Redis client
	client, err := redis.New("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("Failed to create Redis client: %v", err)
	}

	return client, mr
}

// TestNewPublisherAdminHandler tests handler creation
func TestNewPublisherAdminHandler(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)
	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}

	if handler.redisClient == nil {
		t.Error("Expected non-nil Redis client")
	}
}

// TestNewPublisherAdminHandler_NilRedis tests handler creation with nil Redis
func TestNewPublisherAdminHandler_NilRedis(t *testing.T) {
	handler := NewPublisherAdminHandler(nil)
	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}

	if handler.redisClient != nil {
		t.Error("Expected nil Redis client")
	}
}

// TestPublisherAdminHandler_NoRedis tests that endpoints return 503 without Redis
func TestPublisherAdminHandler_NoRedis(t *testing.T) {
	handler := NewPublisherAdminHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/publishers", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "Redis not available" {
		t.Errorf("Expected 'Redis not available' error, got %s", errResp.Error)
	}
}

// TestListPublishers_Empty tests listing with no publishers
func TestListPublishers_Empty(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/admin/publishers", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp PublisherListResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Count != 0 {
		t.Errorf("Expected count 0, got %d", resp.Count)
	}

	if len(resp.Publishers) != 0 {
		t.Errorf("Expected 0 publishers, got %d", len(resp.Publishers))
	}
}

// TestListPublishers_WithData tests listing publishers
func TestListPublishers_WithData(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	// Populate test data
	mr.HSet(publishersHashKey, "pub1", "example.com|test.com")
	mr.HSet(publishersHashKey, "pub2", "*.domain.com")

	handler := NewPublisherAdminHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/admin/publishers", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp PublisherListResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Count != 2 {
		t.Errorf("Expected count 2, got %d", resp.Count)
	}

	if len(resp.Publishers) != 2 {
		t.Errorf("Expected 2 publishers, got %d", len(resp.Publishers))
	}
}

// TestGetPublisher_Success tests getting a specific publisher
func TestGetPublisher_Success(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	// Populate test data
	mr.HSet(publishersHashKey, "pub1", "example.com|test.com")

	handler := NewPublisherAdminHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/admin/publishers/pub1", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var pub Publisher
	err := json.NewDecoder(w.Body).Decode(&pub)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if pub.ID != "pub1" {
		t.Errorf("Expected ID 'pub1', got '%s'", pub.ID)
	}

	if pub.AllowedDomains != "example.com|test.com" {
		t.Errorf("Expected domains 'example.com|test.com', got '%s'", pub.AllowedDomains)
	}

	if len(pub.DomainList) != 2 {
		t.Errorf("Expected 2 domains in list, got %d", len(pub.DomainList))
	}
}

// TestGetPublisher_NotFound tests getting non-existent publisher
func TestGetPublisher_NotFound(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/admin/publishers/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "not_found" {
		t.Errorf("Expected error 'not_found', got '%s'", errResp.Error)
	}
}

// TestCreatePublisher_Success tests creating a new publisher
func TestCreatePublisher_Success(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	reqBody := PublisherRequest{
		ID:             "newpub",
		AllowedDomains: "example.com|test.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var pub Publisher
	err := json.NewDecoder(w.Body).Decode(&pub)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if pub.ID != "newpub" {
		t.Errorf("Expected ID 'newpub', got '%s'", pub.ID)
	}

	// Verify it was actually saved in Redis
	saved := mr.HGet(publishersHashKey, "newpub")
	if saved != "example.com|test.com" {
		t.Errorf("Expected saved domains 'example.com|test.com', got '%s'", saved)
	}
}

// TestCreatePublisher_MissingID tests creating publisher without ID
func TestCreatePublisher_MissingID(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	reqBody := PublisherRequest{
		AllowedDomains: "example.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "missing_id" {
		t.Errorf("Expected error 'missing_id', got '%s'", errResp.Error)
	}
}

// TestCreatePublisher_MissingDomains tests creating publisher without domains
func TestCreatePublisher_MissingDomains(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	reqBody := PublisherRequest{
		ID: "pub1",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "missing_domains" {
		t.Errorf("Expected error 'missing_domains', got '%s'", errResp.Error)
	}
}

// TestCreatePublisher_AlreadyExists tests creating duplicate publisher
func TestCreatePublisher_AlreadyExists(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	// Pre-populate existing publisher
	mr.HSet(publishersHashKey, "existing", "old.com")

	handler := NewPublisherAdminHandler(client)

	reqBody := PublisherRequest{
		ID:             "existing",
		AllowedDomains: "new.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "already_exists" {
		t.Errorf("Expected error 'already_exists', got '%s'", errResp.Error)
	}
}

// TestCreatePublisher_InvalidJSON tests creating publisher with invalid JSON
func TestCreatePublisher_InvalidJSON(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	req := httptest.NewRequest(http.MethodPost, "/admin/publishers", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "invalid_json" {
		t.Errorf("Expected error 'invalid_json', got '%s'", errResp.Error)
	}
}

// TestUpdatePublisher_Success tests updating an existing publisher
func TestUpdatePublisher_Success(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	// Pre-populate existing publisher
	mr.HSet(publishersHashKey, "pub1", "old.com")

	handler := NewPublisherAdminHandler(client)

	reqBody := PublisherRequest{
		AllowedDomains: "new.com|another.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/admin/publishers/pub1", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var pub Publisher
	err := json.NewDecoder(w.Body).Decode(&pub)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if pub.AllowedDomains != "new.com|another.com" {
		t.Errorf("Expected domains 'new.com|another.com', got '%s'", pub.AllowedDomains)
	}

	// Verify it was actually updated in Redis
	saved := mr.HGet(publishersHashKey, "pub1")
	if saved != "new.com|another.com" {
		t.Errorf("Expected saved domains 'new.com|another.com', got '%s'", saved)
	}
}

// TestUpdatePublisher_NotFound tests updating non-existent publisher
func TestUpdatePublisher_NotFound(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	reqBody := PublisherRequest{
		AllowedDomains: "new.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/admin/publishers/nonexistent", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "not_found" {
		t.Errorf("Expected error 'not_found', got '%s'", errResp.Error)
	}
}

// TestUpdatePublisher_MissingID tests updating without ID in path
func TestUpdatePublisher_MissingID(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	reqBody := PublisherRequest{
		AllowedDomains: "new.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/admin/publishers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "missing_publisher_id" {
		t.Errorf("Expected error 'missing_publisher_id', got '%s'", errResp.Error)
	}
}

// TestUpdatePublisher_MissingDomains tests updating without domains
func TestUpdatePublisher_MissingDomains(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	// Pre-populate existing publisher
	mr.HSet(publishersHashKey, "pub1", "old.com")

	handler := NewPublisherAdminHandler(client)

	reqBody := PublisherRequest{
		// No domains
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/admin/publishers/pub1", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "missing_domains" {
		t.Errorf("Expected error 'missing_domains', got '%s'", errResp.Error)
	}
}

// TestDeletePublisher_Success tests deleting a publisher
func TestDeletePublisher_Success(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	// Pre-populate existing publisher
	mr.HSet(publishersHashKey, "pub1", "example.com")

	handler := NewPublisherAdminHandler(client)

	req := httptest.NewRequest(http.MethodDelete, "/admin/publishers/pub1", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if success, ok := resp["success"].(bool); !ok || !success {
		t.Error("Expected success=true in response")
	}

	if pubID, ok := resp["publisher_id"].(string); !ok || pubID != "pub1" {
		t.Errorf("Expected publisher_id='pub1', got '%v'", resp["publisher_id"])
	}

	// Verify it was actually deleted from Redis
	deleted := mr.HGet(publishersHashKey, "pub1")
	if deleted != "" {
		t.Error("Expected publisher to be deleted from Redis")
	}
}

// TestDeletePublisher_NotFound tests deleting non-existent publisher
func TestDeletePublisher_NotFound(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	req := httptest.NewRequest(http.MethodDelete, "/admin/publishers/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "not_found" {
		t.Errorf("Expected error 'not_found', got '%s'", errResp.Error)
	}
}

// TestDeletePublisher_MissingID tests deleting without ID in path
func TestDeletePublisher_MissingID(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	req := httptest.NewRequest(http.MethodDelete, "/admin/publishers", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var errResp ErrorResponse
	err := json.NewDecoder(w.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error != "missing_publisher_id" {
		t.Errorf("Expected error 'missing_publisher_id', got '%s'", errResp.Error)
	}
}

// TestMethodNotAllowed tests unsupported HTTP methods
func TestMethodNotAllowed(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	handler := NewPublisherAdminHandler(client)

	methods := []string{http.MethodPatch, http.MethodHead, http.MethodOptions}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/admin/publishers", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405 for %s, got %d", method, w.Code)
			}

			var errResp ErrorResponse
			err := json.NewDecoder(w.Body).Decode(&errResp)
			if err != nil {
				t.Fatalf("Failed to decode error response: %v", err)
			}

			if errResp.Error != "method_not_allowed" {
				t.Errorf("Expected error 'method_not_allowed', got '%s'", errResp.Error)
			}
		})
	}
}

// TestParseDomains tests domain parsing helper
func TestParseDomains(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Single domain",
			input:    "example.com",
			expected: []string{"example.com"},
		},
		{
			name:     "Multiple domains",
			input:    "example.com|test.com|another.com",
			expected: []string{"example.com", "test.com", "another.com"},
		},
		{
			name:     "Domains with spaces",
			input:    "example.com | test.com | another.com",
			expected: []string{"example.com", "test.com", "another.com"},
		},
		{
			name:     "Wildcard domain",
			input:    "*.example.com",
			expected: []string{"*.example.com"},
		},
		{
			name:     "Empty pipes",
			input:    "example.com||test.com",
			expected: []string{"example.com", "test.com"},
		},
		{
			name:     "Trailing pipe",
			input:    "example.com|test.com|",
			expected: []string{"example.com", "test.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDomains(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d domains, got %d", len(tt.expected), len(result))
			}

			for i, expected := range tt.expected {
				if i >= len(result) {
					t.Errorf("Missing domain at index %d", i)
					continue
				}
				if result[i] != expected {
					t.Errorf("Expected domain[%d]='%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

// TestServeHTTP_PathParsing tests various path formats
func TestServeHTTP_PathParsing(t *testing.T) {
	client, mr := setupTestRedisForPublisher(t)
	defer mr.Close()

	// Pre-populate test data
	mr.HSet(publishersHashKey, "test-pub", "example.com")

	handler := NewPublisherAdminHandler(client)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "Root path",
			path:           "/admin/publishers",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "With trailing slash",
			path:           "/admin/publishers/",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "With publisher ID",
			path:           "/admin/publishers/test-pub",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "With publisher ID and trailing slash",
			path:           "/admin/publishers/test-pub/",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d for path %s, got %d", tt.expectedStatus, tt.path, w.Code)
			}
		})
	}
}
