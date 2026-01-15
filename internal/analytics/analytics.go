// Package analytics provides auction analytics collection and reporting
package analytics

import (
	"context"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// EventType represents different analytics event types
type EventType string

const (
	EventAuctionStart    EventType = "auction_start"
	EventAuctionEnd      EventType = "auction_end"
	EventBidRequest      EventType = "bid_request"
	EventBidResponse     EventType = "bid_response"
	EventNoBid           EventType = "no_bid"
	EventBidWon          EventType = "bid_won"
	EventBidTimeout      EventType = "bid_timeout"
	EventBidError        EventType = "bid_error"
	EventCookieSync      EventType = "cookie_sync"
	EventSetUID          EventType = "set_uid"
	EventFloorEnforced   EventType = "floor_enforced"
	EventPrivacyFiltered EventType = "privacy_filtered"
)

// AuctionEvent contains data about an auction event
type AuctionEvent struct {
	// Event metadata
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id"`

	// Request context
	PublisherID string `json:"publisher_id,omitempty"`
	Domain      string `json:"domain,omitempty"`
	AppBundle   string `json:"app_bundle,omitempty"`

	// Auction details
	Request       *openrtb.BidRequest  `json:"request,omitempty"`
	Response      *openrtb.BidResponse `json:"response,omitempty"`
	BidderCode    string               `json:"bidder_code,omitempty"`
	BidID         string               `json:"bid_id,omitempty"`
	ImpID         string               `json:"imp_id,omitempty"`
	BidPrice      float64              `json:"bid_price,omitempty"`
	BidCurrency   string               `json:"bid_currency,omitempty"`
	DealID        string               `json:"deal_id,omitempty"`

	// Timing
	StartTime time.Time     `json:"start_time,omitempty"`
	EndTime   time.Time     `json:"end_time,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`

	// Error details
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	// Privacy
	GDPRApplies   bool   `json:"gdpr_applies,omitempty"`
	ConsentString string `json:"consent_string,omitempty"`

	// Additional data
	Extra map[string]interface{} `json:"extra,omitempty"`
}

// Adapter is the interface for analytics backends
type Adapter interface {
	// LogAuctionEvent logs an auction-related event
	LogAuctionEvent(ctx context.Context, event *AuctionEvent) error

	// Name returns the adapter name for logging
	Name() string

	// Close gracefully shuts down the adapter
	Close() error
}

// Engine manages multiple analytics adapters
type Engine struct {
	mu       sync.RWMutex
	adapters []Adapter
	config   *Config
	eventCh  chan *AuctionEvent
	done     chan struct{}
}

// Config holds analytics engine configuration
type Config struct {
	// Enabled controls whether analytics is active
	Enabled bool `json:"enabled"`

	// BufferSize for the event channel
	BufferSize int `json:"buffer_size"`

	// Workers for concurrent event processing
	Workers int `json:"workers"`

	// IncludeFullRequest includes complete request in events
	IncludeFullRequest bool `json:"include_full_request"`

	// IncludeFullResponse includes complete response in events
	IncludeFullResponse bool `json:"include_full_response"`

	// SampleRate for event sampling (0.0-1.0, 1.0 = all events)
	SampleRate float64 `json:"sample_rate"`
}

// DefaultConfig returns production-safe defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:             true,
		BufferSize:          10000,
		Workers:             4,
		IncludeFullRequest:  false,
		IncludeFullResponse: false,
		SampleRate:          1.0,
	}
}

// NewEngine creates a new analytics engine
func NewEngine(config *Config) *Engine {
	if config == nil {
		config = DefaultConfig()
	}

	e := &Engine{
		adapters: make([]Adapter, 0),
		config:   config,
		eventCh:  make(chan *AuctionEvent, config.BufferSize),
		done:     make(chan struct{}),
	}

	// Start workers
	for i := 0; i < config.Workers; i++ {
		go e.worker()
	}

	return e
}

// AddAdapter registers an analytics adapter
func (e *Engine) AddAdapter(adapter Adapter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.adapters = append(e.adapters, adapter)
}

// RemoveAdapter removes an adapter by name
func (e *Engine) RemoveAdapter(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, a := range e.adapters {
		if a.Name() == name {
			e.adapters = append(e.adapters[:i], e.adapters[i+1:]...)
			return
		}
	}
}

// LogEvent queues an event for processing
func (e *Engine) LogEvent(event *AuctionEvent) {
	if !e.config.Enabled {
		return
	}

	// Sample rate check
	if e.config.SampleRate < 1.0 {
		// Simple deterministic sampling based on request ID
		if !shouldSample(event.RequestID, e.config.SampleRate) {
			return
		}
	}

	// Non-blocking send
	select {
	case e.eventCh <- event:
	default:
		// Channel full, drop event
	}
}

// LogAuctionStart logs the start of an auction
func (e *Engine) LogAuctionStart(req *openrtb.BidRequest) {
	event := &AuctionEvent{
		Type:      EventAuctionStart,
		Timestamp: time.Now(),
		RequestID: req.ID,
		StartTime: time.Now(),
	}

	if req.Site != nil {
		event.Domain = req.Site.Domain
		if req.Site.Publisher != nil {
			event.PublisherID = req.Site.Publisher.ID
		}
	} else if req.App != nil {
		event.AppBundle = req.App.Bundle
		if req.App.Publisher != nil {
			event.PublisherID = req.App.Publisher.ID
		}
	}

	if e.config.IncludeFullRequest {
		event.Request = req
	}

	e.LogEvent(event)
}

// LogAuctionEnd logs the end of an auction
func (e *Engine) LogAuctionEnd(req *openrtb.BidRequest, resp *openrtb.BidResponse, duration time.Duration) {
	event := &AuctionEvent{
		Type:      EventAuctionEnd,
		Timestamp: time.Now(),
		RequestID: req.ID,
		Duration:  duration,
	}

	if req.Site != nil {
		event.Domain = req.Site.Domain
		if req.Site.Publisher != nil {
			event.PublisherID = req.Site.Publisher.ID
		}
	} else if req.App != nil {
		event.AppBundle = req.App.Bundle
		if req.App.Publisher != nil {
			event.PublisherID = req.App.Publisher.ID
		}
	}

	if e.config.IncludeFullRequest {
		event.Request = req
	}
	if e.config.IncludeFullResponse {
		event.Response = resp
	}

	e.LogEvent(event)
}

// LogBidRequest logs a bid request to a bidder
func (e *Engine) LogBidRequest(requestID, bidderCode string, req *openrtb.BidRequest) {
	event := &AuctionEvent{
		Type:       EventBidRequest,
		Timestamp:  time.Now(),
		RequestID:  requestID,
		BidderCode: bidderCode,
		StartTime:  time.Now(),
	}

	if e.config.IncludeFullRequest {
		event.Request = req
	}

	e.LogEvent(event)
}

// LogBidResponse logs a bid response from a bidder
func (e *Engine) LogBidResponse(requestID, bidderCode string, bids []openrtb.Bid, duration time.Duration) {
	for _, bid := range bids {
		event := &AuctionEvent{
			Type:        EventBidResponse,
			Timestamp:   time.Now(),
			RequestID:   requestID,
			BidderCode:  bidderCode,
			BidID:       bid.ID,
			ImpID:       bid.ImpID,
			BidPrice:    bid.Price,
			DealID:      bid.DealID,
			Duration:    duration,
		}
		e.LogEvent(event)
	}
}

// LogNoBid logs a no-bid from a bidder
func (e *Engine) LogNoBid(requestID, bidderCode, reason string) {
	event := &AuctionEvent{
		Type:         EventNoBid,
		Timestamp:    time.Now(),
		RequestID:    requestID,
		BidderCode:   bidderCode,
		ErrorMessage: reason,
	}
	e.LogEvent(event)
}

// LogBidWon logs a winning bid
func (e *Engine) LogBidWon(requestID, bidderCode, bidID, impID string, price float64) {
	event := &AuctionEvent{
		Type:       EventBidWon,
		Timestamp:  time.Now(),
		RequestID:  requestID,
		BidderCode: bidderCode,
		BidID:      bidID,
		ImpID:      impID,
		BidPrice:   price,
	}
	e.LogEvent(event)
}

// LogBidTimeout logs a bidder timeout
func (e *Engine) LogBidTimeout(requestID, bidderCode string, duration time.Duration) {
	event := &AuctionEvent{
		Type:       EventBidTimeout,
		Timestamp:  time.Now(),
		RequestID:  requestID,
		BidderCode: bidderCode,
		Duration:   duration,
	}
	e.LogEvent(event)
}

// LogBidError logs a bidder error
func (e *Engine) LogBidError(requestID, bidderCode, errorCode, errorMessage string) {
	event := &AuctionEvent{
		Type:         EventBidError,
		Timestamp:    time.Now(),
		RequestID:    requestID,
		BidderCode:   bidderCode,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	}
	e.LogEvent(event)
}

// LogCookieSync logs a cookie sync event
func (e *Engine) LogCookieSync(bidders []string, syncsReturned int) {
	event := &AuctionEvent{
		Type:      EventCookieSync,
		Timestamp: time.Now(),
		Extra: map[string]interface{}{
			"bidders_requested": bidders,
			"syncs_returned":    syncsReturned,
		},
	}
	e.LogEvent(event)
}

// worker processes events from the channel
func (e *Engine) worker() {
	for {
		select {
		case event := <-e.eventCh:
			e.processEvent(event)
		case <-e.done:
			return
		}
	}
}

// processEvent sends event to all adapters
func (e *Engine) processEvent(event *AuctionEvent) {
	e.mu.RLock()
	adapters := e.adapters
	e.mu.RUnlock()

	ctx := context.Background()
	for _, adapter := range adapters {
		// Fire and forget - don't block on adapter errors
		go func(a Adapter) {
			_ = a.LogAuctionEvent(ctx, event)
		}(adapter)
	}
}

// Close shuts down the analytics engine
func (e *Engine) Close() error {
	close(e.done)

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, adapter := range e.adapters {
		_ = adapter.Close()
	}

	return nil
}

// GetConfig returns current configuration
func (e *Engine) GetConfig() *Config {
	return e.config
}

// SetEnabled enables/disables analytics
func (e *Engine) SetEnabled(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.Enabled = enabled
}

// shouldSample determines if an event should be sampled
func shouldSample(requestID string, rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}

	// Simple hash-based sampling
	var hash uint32
	for i := 0; i < len(requestID); i++ {
		hash = hash*31 + uint32(requestID[i])
	}
	return float64(hash%1000)/1000.0 < rate
}
