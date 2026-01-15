// Package cache provides bid caching for Prebid Cache integration
package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// Client handles bid caching with Prebid Cache
type Client struct {
	mu         sync.RWMutex
	endpoint   string
	httpClient *http.Client
	config     *Config
	localCache map[string]*cacheEntry
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// Config holds cache client configuration
type Config struct {
	// Enabled controls whether caching is active
	Enabled bool `json:"enabled"`

	// Endpoint is the Prebid Cache URL (e.g., https://prebid.example.com/cache)
	Endpoint string `json:"endpoint"`

	// Timeout for cache requests
	Timeout time.Duration `json:"timeout"`

	// DefaultTTL for cached items
	DefaultTTL time.Duration `json:"default_ttl"`

	// UseLocalCache enables local in-memory caching
	UseLocalCache bool `json:"use_local_cache"`

	// LocalCacheSize maximum entries in local cache
	LocalCacheSize int `json:"local_cache_size"`
}

// DefaultConfig returns production-safe defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:        true,
		Timeout:        100 * time.Millisecond,
		DefaultTTL:     5 * time.Minute,
		UseLocalCache:  true,
		LocalCacheSize: 10000,
	}
}

// CacheRequest is the request format for Prebid Cache
type CacheRequest struct {
	Puts []CachePut `json:"puts"`
}

// CachePut represents a single item to cache
type CachePut struct {
	Type  string `json:"type"`            // "json" or "xml"
	Value string `json:"value,omitempty"` // For JSON
	Key   string `json:"key,omitempty"`   // Optional custom key
	TTL   int    `json:"ttlseconds,omitempty"`
}

// CacheResponse is the response format from Prebid Cache
type CacheResponse struct {
	Responses []CacheResponseItem `json:"responses"`
}

// CacheResponseItem is a single cached item response
type CacheResponseItem struct {
	UUID string `json:"uuid"`
}

// BidCache represents a cached bid
type BidCache struct {
	UUID     string `json:"uuid"`
	CacheURL string `json:"cache_url,omitempty"`
	CacheID  string `json:"cache_id,omitempty"`
}

// NewClient creates a new cache client
func NewClient(config *Config) *Client {
	if config == nil {
		config = DefaultConfig()
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}

	c := &Client{
		endpoint:   config.Endpoint,
		httpClient: &http.Client{Timeout: timeout},
		config:     config,
	}

	if config.UseLocalCache {
		c.localCache = make(map[string]*cacheEntry, config.LocalCacheSize)
	}

	return c
}

// StoreBids caches multiple bid values and returns their UUIDs
func (c *Client) StoreBids(ctx context.Context, bids []string) ([]BidCache, error) {
	if !c.config.Enabled || c.endpoint == "" {
		return nil, nil
	}

	puts := make([]CachePut, len(bids))
	for i, bid := range bids {
		puts[i] = CachePut{
			Type:  "json",
			Value: bid,
			TTL:   int(c.config.DefaultTTL.Seconds()),
		}
	}

	return c.store(ctx, puts)
}

// StoreVAST caches VAST XML and returns the UUID
func (c *Client) StoreVAST(ctx context.Context, vast string, ttl time.Duration) (*BidCache, error) {
	if !c.config.Enabled || c.endpoint == "" {
		return nil, nil
	}

	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	puts := []CachePut{{
		Type:  "xml",
		Value: vast,
		TTL:   int(ttl.Seconds()),
	}}

	results, err := c.store(ctx, puts)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no cache response")
	}

	return &results[0], nil
}

// store sends items to Prebid Cache
func (c *Client) store(ctx context.Context, puts []CachePut) ([]BidCache, error) {
	reqBody := CacheRequest{Puts: puts}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Log.Debug().Err(err).Msg("Failed to store in Prebid Cache")
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cache returned status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var cacheResp CacheResponse
	if err := json.Unmarshal(respBody, &cacheResp); err != nil {
		return nil, err
	}

	results := make([]BidCache, len(cacheResp.Responses))
	for i, item := range cacheResp.Responses {
		results[i] = BidCache{
			UUID:     item.UUID,
			CacheURL: c.buildCacheURL(item.UUID),
			CacheID:  item.UUID,
		}
	}

	// Store in local cache if enabled
	if c.config.UseLocalCache {
		c.mu.Lock()
		for i, put := range puts {
			if i < len(results) {
				c.localCache[results[i].UUID] = &cacheEntry{
					value:     put.Value,
					expiresAt: time.Now().Add(time.Duration(put.TTL) * time.Second),
				}
			}
		}
		c.mu.Unlock()
	}

	return results, nil
}

// Get retrieves a cached item by UUID
func (c *Client) Get(ctx context.Context, uuid string) (string, error) {
	// Check local cache first
	if c.config.UseLocalCache {
		c.mu.RLock()
		entry, ok := c.localCache[uuid]
		c.mu.RUnlock()

		if ok && time.Now().Before(entry.expiresAt) {
			return entry.value, nil
		}
	}

	if c.endpoint == "" {
		return "", fmt.Errorf("cache endpoint not configured")
	}

	// Fetch from Prebid Cache
	url := c.endpoint + "?uuid=" + uuid
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cache returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// buildCacheURL constructs the full cache URL for a UUID
func (c *Client) buildCacheURL(uuid string) string {
	if c.endpoint == "" {
		return ""
	}
	return c.endpoint + "?uuid=" + uuid
}

// SetEndpoint updates the cache endpoint
func (c *Client) SetEndpoint(endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.endpoint = endpoint
}

// SetEnabled enables/disables caching
func (c *Client) SetEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.Enabled = enabled
}

// ClearLocalCache clears the local cache
func (c *Client) ClearLocalCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localCache = make(map[string]*cacheEntry, c.config.LocalCacheSize)
}

// GetConfig returns current configuration
func (c *Client) GetConfig() *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// LocalCacheStats returns statistics about the local cache
func (c *Client) LocalCacheStats() (size int, expired int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	for _, entry := range c.localCache {
		if now.After(entry.expiresAt) {
			expired++
		}
	}
	return len(c.localCache), expired
}
