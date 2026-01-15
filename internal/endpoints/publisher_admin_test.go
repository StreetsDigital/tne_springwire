package endpoints

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewPublisherAdminHandler(t *testing.T) {
	handler := NewPublisherAdminHandler(nil)
	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestPublisherAdminHandler_NoRedis(t *testing.T) {
	handler := NewPublisherAdminHandler(nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list publishers", http.MethodGet, "/admin/publishers"},
		{"get publisher", http.MethodGet, "/admin/publishers/pub-1"},
		{"create publisher", http.MethodPost, "/admin/publishers"},
		{"update publisher", http.MethodPut, "/admin/publishers/pub-1"},
		{"delete publisher", http.MethodDelete, "/admin/publishers/pub-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// Should return service unavailable when Redis is nil
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("expected status 503, got %d", rec.Code)
			}

			// Should return JSON error
			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", contentType)
			}
		})
	}
}

func TestPublisherAdminHandler_UnsupportedMethod(t *testing.T) {
	handler := NewPublisherAdminHandler(nil)

	req := httptest.NewRequest(http.MethodPatch, "/admin/publishers", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// First it will hit the Redis check, so status will be 503
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 for nil redis, got %d", rec.Code)
	}
}

func TestParseDomains(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", []string{}},
		{"domain.com", []string{"domain.com"}},
		{"domain1.com|domain2.com", []string{"domain1.com", "domain2.com"}},
		{"*.domain.com|exact.com", []string{"*.domain.com", "exact.com"}},
		{"  domain.com  |  other.com  ", []string{"domain.com", "other.com"}},
	}

	for _, tt := range tests {
		result := parseDomains(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseDomains(%q) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("parseDomains(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestSendJSON(t *testing.T) {
	handler := &PublisherAdminHandler{}
	rec := httptest.NewRecorder()

	data := map[string]string{"key": "value"}
	handler.sendJSON(rec, http.StatusOK, data)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	if !strings.Contains(rec.Body.String(), `"key":"value"`) {
		t.Error("expected JSON body to contain key:value")
	}
}

func TestSendError(t *testing.T) {
	handler := &PublisherAdminHandler{}
	rec := httptest.NewRecorder()

	handler.sendError(rec, http.StatusBadRequest, "bad request", "missing required field")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "bad request") {
		t.Error("expected body to contain error message")
	}
}

func TestPublisherStruct(t *testing.T) {
	p := Publisher{
		ID:             "pub-1",
		AllowedDomains: "domain1.com|domain2.com",
		DomainList:     []string{"domain1.com", "domain2.com"},
	}

	if p.ID != "pub-1" {
		t.Error("ID not set correctly")
	}
	if len(p.DomainList) != 2 {
		t.Error("DomainList not set correctly")
	}
}

func TestPublisherRequest(t *testing.T) {
	req := PublisherRequest{
		ID:             "pub-2",
		AllowedDomains: "example.com|*.test.com",
	}

	if req.ID != "pub-2" {
		t.Error("ID not set correctly")
	}
	if req.AllowedDomains != "example.com|*.test.com" {
		t.Error("AllowedDomains not set correctly")
	}
}

func TestErrorResponse(t *testing.T) {
	resp := ErrorResponse{
		Error:   "not found",
		Message: "The requested resource was not found",
	}

	if resp.Error != "not found" {
		t.Error("Error not set correctly")
	}
	if resp.Message != "The requested resource was not found" {
		t.Error("Message not set correctly")
	}
}
