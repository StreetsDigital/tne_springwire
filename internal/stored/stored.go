// Package stored provides functionality for managing stored requests and responses
// Stored requests allow publishers to pre-configure bid request templates server-side,
// reducing payload size and latency while enabling centralized configuration management.
package stored

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// Errors
var (
	ErrNotFound        = errors.New("stored data not found")
	ErrInvalidJSON     = errors.New("stored data is not valid JSON")
	ErrFetcherClosed   = errors.New("fetcher is closed")
	ErrMergeConflict   = errors.New("merge conflict between stored and incoming data")
	ErrInvalidStoredID = errors.New("invalid stored request ID")
)

// DataType represents the type of stored data
type DataType string

const (
	DataTypeRequest    DataType = "request"    // Stored bid request
	DataTypeImpression DataType = "impression" // Stored impression
	DataTypeResponse   DataType = "response"   // Stored response (for testing)
	DataTypeAccount    DataType = "account"    // Stored account config
)

// StoredData represents a stored request, impression, or response
type StoredData struct {
	// ID is the unique identifier for this stored data
	ID string `json:"id"`
	// Type indicates what kind of stored data this is
	Type DataType `json:"type"`
	// Data is the raw JSON data
	Data json.RawMessage `json:"data"`
	// CreatedAt is when this data was created
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this data was last updated
	UpdatedAt time.Time `json:"updated_at"`
	// AccountID is the account this data belongs to (optional)
	AccountID string `json:"account_id,omitempty"`
	// Disabled indicates if this stored data is disabled
	Disabled bool `json:"disabled,omitempty"`
}

// Fetcher is the interface for retrieving stored data
type Fetcher interface {
	// FetchRequests retrieves stored request data for the given IDs
	// Returns a map of ID -> JSON data
	FetchRequests(ctx context.Context, requestIDs []string) (map[string]json.RawMessage, []error)

	// FetchImpressions retrieves stored impression data for the given IDs
	// Returns a map of ID -> JSON data
	FetchImpressions(ctx context.Context, impIDs []string) (map[string]json.RawMessage, []error)

	// FetchResponses retrieves stored response data for the given IDs (for testing)
	FetchResponses(ctx context.Context, respIDs []string) (map[string]json.RawMessage, []error)

	// FetchAccount retrieves stored account configuration
	FetchAccount(ctx context.Context, accountID string) (json.RawMessage, error)

	// Close releases resources
	Close() error
}

// Cache wraps a Fetcher with caching capabilities
type Cache struct {
	backend     Fetcher
	requests    *sync.Map
	impressions *sync.Map
	responses   *sync.Map
	accounts    *sync.Map
	ttl         time.Duration
	mu          sync.RWMutex
	closed      bool
}

// cacheEntry holds cached data with expiration
type cacheEntry struct {
	data      json.RawMessage
	expiresAt time.Time
}

// CacheConfig configures the cache behavior
type CacheConfig struct {
	// TTL is the time-to-live for cached entries
	TTL time.Duration
	// MaxEntries is the maximum number of entries to cache (0 = unlimited)
	MaxEntries int
}

// DefaultCacheConfig returns sensible defaults
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:        5 * time.Minute,
		MaxEntries: 10000,
	}
}

// NewCache creates a new caching wrapper around a Fetcher
func NewCache(backend Fetcher, config CacheConfig) *Cache {
	return &Cache{
		backend:     backend,
		requests:    &sync.Map{},
		impressions: &sync.Map{},
		responses:   &sync.Map{},
		accounts:    &sync.Map{},
		ttl:         config.TTL,
	}
}

// FetchRequests implements Fetcher with caching
func (c *Cache) FetchRequests(ctx context.Context, requestIDs []string) (map[string]json.RawMessage, []error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, []error{ErrFetcherClosed}
	}
	c.mu.RUnlock()

	result := make(map[string]json.RawMessage)
	var missing []string
	var errs []error

	// Check cache first
	for _, id := range requestIDs {
		if entry, ok := c.requests.Load(id); ok {
			ce := entry.(*cacheEntry)
			if time.Now().Before(ce.expiresAt) {
				result[id] = ce.data
				continue
			}
			// Expired, remove from cache
			c.requests.Delete(id)
		}
		missing = append(missing, id)
	}

	// Fetch missing from backend
	if len(missing) > 0 {
		fetched, fetchErrs := c.backend.FetchRequests(ctx, missing)
		errs = append(errs, fetchErrs...)

		// Cache fetched results
		for id, data := range fetched {
			c.requests.Store(id, &cacheEntry{
				data:      data,
				expiresAt: time.Now().Add(c.ttl),
			})
			result[id] = data
		}
	}

	return result, errs
}

// FetchImpressions implements Fetcher with caching
func (c *Cache) FetchImpressions(ctx context.Context, impIDs []string) (map[string]json.RawMessage, []error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, []error{ErrFetcherClosed}
	}
	c.mu.RUnlock()

	result := make(map[string]json.RawMessage)
	var missing []string
	var errs []error

	for _, id := range impIDs {
		if entry, ok := c.impressions.Load(id); ok {
			ce := entry.(*cacheEntry)
			if time.Now().Before(ce.expiresAt) {
				result[id] = ce.data
				continue
			}
			c.impressions.Delete(id)
		}
		missing = append(missing, id)
	}

	if len(missing) > 0 {
		fetched, fetchErrs := c.backend.FetchImpressions(ctx, missing)
		errs = append(errs, fetchErrs...)

		for id, data := range fetched {
			c.impressions.Store(id, &cacheEntry{
				data:      data,
				expiresAt: time.Now().Add(c.ttl),
			})
			result[id] = data
		}
	}

	return result, errs
}

// FetchResponses implements Fetcher with caching
func (c *Cache) FetchResponses(ctx context.Context, respIDs []string) (map[string]json.RawMessage, []error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, []error{ErrFetcherClosed}
	}
	c.mu.RUnlock()

	result := make(map[string]json.RawMessage)
	var missing []string
	var errs []error

	for _, id := range respIDs {
		if entry, ok := c.responses.Load(id); ok {
			ce := entry.(*cacheEntry)
			if time.Now().Before(ce.expiresAt) {
				result[id] = ce.data
				continue
			}
			c.responses.Delete(id)
		}
		missing = append(missing, id)
	}

	if len(missing) > 0 {
		fetched, fetchErrs := c.backend.FetchResponses(ctx, missing)
		errs = append(errs, fetchErrs...)

		for id, data := range fetched {
			c.responses.Store(id, &cacheEntry{
				data:      data,
				expiresAt: time.Now().Add(c.ttl),
			})
			result[id] = data
		}
	}

	return result, errs
}

// FetchAccount implements Fetcher with caching
func (c *Cache) FetchAccount(ctx context.Context, accountID string) (json.RawMessage, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrFetcherClosed
	}
	c.mu.RUnlock()

	if entry, ok := c.accounts.Load(accountID); ok {
		ce := entry.(*cacheEntry)
		if time.Now().Before(ce.expiresAt) {
			return ce.data, nil
		}
		c.accounts.Delete(accountID)
	}

	data, err := c.backend.FetchAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	c.accounts.Store(accountID, &cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	})

	return data, nil
}

// Close releases resources
func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	return c.backend.Close()
}

// Invalidate removes specific entries from the cache
func (c *Cache) Invalidate(dataType DataType, ids []string) {
	var cache *sync.Map
	switch dataType {
	case DataTypeRequest:
		cache = c.requests
	case DataTypeImpression:
		cache = c.impressions
	case DataTypeResponse:
		cache = c.responses
	case DataTypeAccount:
		cache = c.accounts
	default:
		return
	}

	for _, id := range ids {
		cache.Delete(id)
	}
}

// InvalidateAll clears the entire cache
func (c *Cache) InvalidateAll() {
	c.requests = &sync.Map{}
	c.impressions = &sync.Map{}
	c.responses = &sync.Map{}
	c.accounts = &sync.Map{}
}

// Stats returns cache statistics
func (c *Cache) Stats() CacheStats {
	var stats CacheStats

	c.requests.Range(func(_, _ interface{}) bool {
		stats.RequestCount++
		return true
	})
	c.impressions.Range(func(_, _ interface{}) bool {
		stats.ImpressionCount++
		return true
	})
	c.responses.Range(func(_, _ interface{}) bool {
		stats.ResponseCount++
		return true
	})
	c.accounts.Range(func(_, _ interface{}) bool {
		stats.AccountCount++
		return true
	})

	return stats
}

// CacheStats holds cache statistics
type CacheStats struct {
	RequestCount    int `json:"request_count"`
	ImpressionCount int `json:"impression_count"`
	ResponseCount   int `json:"response_count"`
	AccountCount    int `json:"account_count"`
}

// ExtractStoredRequestID extracts the stored request ID from ext.prebid.storedrequest.id
func ExtractStoredRequestID(ext json.RawMessage) (string, error) {
	if ext == nil {
		return "", nil
	}

	var extData struct {
		Prebid struct {
			StoredRequest struct {
				ID string `json:"id"`
			} `json:"storedrequest"`
		} `json:"prebid"`
	}

	if err := json.Unmarshal(ext, &extData); err != nil {
		return "", nil // Not an error if ext doesn't have storedrequest
	}

	return extData.Prebid.StoredRequest.ID, nil
}

// ExtractStoredImpID extracts the stored impression ID from imp.ext.prebid.storedrequest.id
func ExtractStoredImpID(ext json.RawMessage) (string, error) {
	if ext == nil {
		return "", nil
	}

	var extData struct {
		Prebid struct {
			StoredRequest struct {
				ID string `json:"id"`
			} `json:"storedrequest"`
		} `json:"prebid"`
	}

	if err := json.Unmarshal(ext, &extData); err != nil {
		return "", nil
	}

	return extData.Prebid.StoredRequest.ID, nil
}

// MergeResult contains the result of merging stored and incoming data
type MergeResult struct {
	// MergedData is the final merged JSON
	MergedData json.RawMessage
	// StoredRequestID is the ID of the stored request that was used
	StoredRequestID string
	// StoredImpIDs maps impression IDs to their stored data IDs
	StoredImpIDs map[string]string
	// Warnings contains non-fatal issues encountered during merge
	Warnings []string
}

// Merger handles merging stored data with incoming requests
type Merger struct {
	fetcher Fetcher
}

// NewMerger creates a new Merger
func NewMerger(fetcher Fetcher) *Merger {
	return &Merger{fetcher: fetcher}
}

// MergeRequest merges stored request data with an incoming request
// The incoming request takes precedence over stored data for conflicting fields
func (m *Merger) MergeRequest(ctx context.Context, incoming json.RawMessage) (*MergeResult, error) {
	result := &MergeResult{
		StoredImpIDs: make(map[string]string),
	}

	// Parse incoming to get stored request ID
	var incomingMap map[string]interface{}
	if err := json.Unmarshal(incoming, &incomingMap); err != nil {
		return nil, fmt.Errorf("invalid incoming JSON: %w", err)
	}

	// Extract stored request ID from ext.prebid.storedrequest.id
	var storedReqID string
	if ext, ok := incomingMap["ext"]; ok {
		if extJSON, err := json.Marshal(ext); err == nil {
			storedReqID, _ = ExtractStoredRequestID(extJSON)
		}
	}

	// If no stored request ID, just return incoming as-is
	if storedReqID == "" {
		result.MergedData = incoming
		return result, nil
	}

	result.StoredRequestID = storedReqID

	// Fetch stored request
	storedData, errs := m.fetcher.FetchRequests(ctx, []string{storedReqID})
	if len(errs) > 0 {
		for _, err := range errs {
			if !errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("failed to fetch stored request %s: %w", storedReqID, err)
			}
		}
	}

	stored, ok := storedData[storedReqID]
	if !ok {
		return nil, fmt.Errorf("stored request not found: %s", storedReqID)
	}

	// Parse stored data
	var storedMap map[string]interface{}
	if err := json.Unmarshal(stored, &storedMap); err != nil {
		return nil, fmt.Errorf("invalid stored JSON for %s: %w", storedReqID, err)
	}

	// Merge: stored as base, incoming overwrites
	merged := deepMerge(storedMap, incomingMap)

	// Handle impressions specially - they need to merge per-impression
	if imps, ok := incomingMap["imp"].([]interface{}); ok {
		mergedImps, impWarnings, err := m.mergeImpressions(ctx, imps, storedMap)
		if err != nil {
			return nil, err
		}
		merged["imp"] = mergedImps
		result.Warnings = append(result.Warnings, impWarnings...)
	}

	// Marshal merged result
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged request: %w", err)
	}

	result.MergedData = mergedJSON

	logger.Log.Debug().
		Str("stored_request_id", storedReqID).
		Int("stored_imp_count", len(result.StoredImpIDs)).
		Msg("Merged stored request with incoming")

	return result, nil
}

// mergeImpressions handles merging impressions with their stored data
func (m *Merger) mergeImpressions(ctx context.Context, incomingImps []interface{}, storedReq map[string]interface{}) ([]interface{}, []string, error) {
	var warnings []string
	var storedImpIDs []string
	impIDMap := make(map[int]string) // Index -> stored ID

	// First pass: collect stored impression IDs
	for i, imp := range incomingImps {
		impMap, ok := imp.(map[string]interface{})
		if !ok {
			continue
		}

		if ext, ok := impMap["ext"]; ok {
			if extJSON, err := json.Marshal(ext); err == nil {
				if storedID, _ := ExtractStoredImpID(extJSON); storedID != "" {
					storedImpIDs = append(storedImpIDs, storedID)
					impIDMap[i] = storedID
				}
			}
		}
	}

	// Fetch all stored impressions at once
	var storedImps map[string]json.RawMessage
	if len(storedImpIDs) > 0 {
		var errs []error
		storedImps, errs = m.fetcher.FetchImpressions(ctx, storedImpIDs)
		for _, err := range errs {
			if !errors.Is(err, ErrNotFound) {
				return nil, nil, err
			}
			warnings = append(warnings, fmt.Sprintf("stored impression fetch error: %v", err))
		}
	}

	// Second pass: merge each impression
	result := make([]interface{}, len(incomingImps))
	for i, imp := range incomingImps {
		impMap, ok := imp.(map[string]interface{})
		if !ok {
			result[i] = imp
			continue
		}

		// Check if this impression has stored data
		if storedID, hasStored := impIDMap[i]; hasStored {
			if storedImp, found := storedImps[storedID]; found {
				var storedImpMap map[string]interface{}
				if err := json.Unmarshal(storedImp, &storedImpMap); err != nil {
					warnings = append(warnings, fmt.Sprintf("invalid stored impression JSON for %s", storedID))
					result[i] = imp
					continue
				}

				// Merge: stored as base, incoming overwrites
				result[i] = deepMerge(storedImpMap, impMap)
			} else {
				warnings = append(warnings, fmt.Sprintf("stored impression not found: %s", storedID))
				result[i] = imp
			}
		} else {
			result[i] = imp
		}
	}

	return result, warnings, nil
}

// deepMerge merges two maps, with src taking precedence over dst for conflicting keys
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all from dst
	for k, v := range dst {
		result[k] = v
	}

	// Merge/overwrite with src
	for k, srcVal := range src {
		if dstVal, exists := result[k]; exists {
			// Both have this key - check if we can deep merge
			srcMap, srcIsMap := srcVal.(map[string]interface{})
			dstMap, dstIsMap := dstVal.(map[string]interface{})

			if srcIsMap && dstIsMap {
				// Both are maps - recurse
				result[k] = deepMerge(dstMap, srcMap)
			} else {
				// Not both maps - src wins
				result[k] = srcVal
			}
		} else {
			// Only in src
			result[k] = srcVal
		}
	}

	return result
}
