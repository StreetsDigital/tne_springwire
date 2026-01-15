// Package bidadjustment provides bid price adjustment functionality
package bidadjustment

import (
	"sync"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// AdjustmentType represents different adjustment types
type AdjustmentType string

const (
	// AdjustmentMultiplier multiplies the bid price
	AdjustmentMultiplier AdjustmentType = "multiplier"
	// AdjustmentCPM adds/subtracts a fixed CPM value
	AdjustmentCPM AdjustmentType = "cpm"
	// AdjustmentStatic sets a static price
	AdjustmentStatic AdjustmentType = "static"
)

// Rule represents a bid adjustment rule
type Rule struct {
	// Matching criteria (all non-empty fields must match)
	Bidder      string `json:"bidder,omitempty"`       // Bidder code
	MediaType   string `json:"media_type,omitempty"`   // banner, video, native, audio
	DealID      string `json:"deal_id,omitempty"`      // Specific deal
	PublisherID string `json:"publisher_id,omitempty"` // Publisher

	// Adjustment
	Type  AdjustmentType `json:"type"`
	Value float64        `json:"value"`

	// Priority for rule ordering (higher = applied first)
	Priority int `json:"priority,omitempty"`

	// Enabled allows temporarily disabling rules
	Enabled bool `json:"enabled"`
}

// Adjuster applies bid adjustments based on configured rules
type Adjuster struct {
	mu     sync.RWMutex
	rules  []Rule
	config *Config
}

// Config holds adjuster configuration
type Config struct {
	// Enabled controls whether adjustments are applied
	Enabled bool `json:"enabled"`

	// MaxAdjustment caps the maximum multiplier (e.g., 2.0 = max 2x)
	MaxAdjustment float64 `json:"max_adjustment"`

	// MinAdjustment caps the minimum multiplier (e.g., 0.5 = min 0.5x)
	MinAdjustment float64 `json:"min_adjustment"`

	// AllowNegative allows adjustments that result in negative prices
	AllowNegative bool `json:"allow_negative"`
}

// DefaultConfig returns production-safe defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:       true,
		MaxAdjustment: 5.0,  // Max 5x increase
		MinAdjustment: 0.1,  // Min 10% of original
		AllowNegative: false,
	}
}

// NewAdjuster creates a new bid adjuster
func NewAdjuster(config *Config) *Adjuster {
	if config == nil {
		config = DefaultConfig()
	}

	return &Adjuster{
		rules:  make([]Rule, 0),
		config: config,
	}
}

// AddRule adds a bid adjustment rule
func (a *Adjuster) AddRule(rule Rule) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Insert in priority order (highest first)
	inserted := false
	for i, r := range a.rules {
		if rule.Priority > r.Priority {
			a.rules = append(a.rules[:i], append([]Rule{rule}, a.rules[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		a.rules = append(a.rules, rule)
	}
}

// RemoveRule removes rules matching the criteria
func (a *Adjuster) RemoveRule(bidder, mediaType string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	filtered := make([]Rule, 0, len(a.rules))
	for _, rule := range a.rules {
		if rule.Bidder != bidder || rule.MediaType != mediaType {
			filtered = append(filtered, rule)
		}
	}
	a.rules = filtered
}

// SetRules replaces all rules
func (a *Adjuster) SetRules(rules []Rule) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Sort by priority (highest first)
	sorted := make([]Rule, len(rules))
	copy(sorted, rules)

	// Simple insertion sort by priority
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Priority > sorted[j-1].Priority; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	a.rules = sorted
}

// ClearRules removes all rules
func (a *Adjuster) ClearRules() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rules = make([]Rule, 0)
}

// GetRules returns a copy of all rules
func (a *Adjuster) GetRules() []Rule {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]Rule, len(a.rules))
	copy(result, a.rules)
	return result
}

// AdjustBid adjusts a single bid based on matching rules
func (a *Adjuster) AdjustBid(bid *openrtb.Bid, bidderCode, mediaType, publisherID string) float64 {
	if !a.config.Enabled {
		return bid.Price
	}

	originalPrice := bid.Price
	adjustedPrice := originalPrice

	a.mu.RLock()
	rules := a.rules
	a.mu.RUnlock()

	// Apply all matching rules in priority order
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		if !a.ruleMatches(&rule, bidderCode, mediaType, bid.DealID, publisherID) {
			continue
		}

		adjustedPrice = a.applyAdjustment(adjustedPrice, &rule)
	}

	// Apply bounds
	adjustedPrice = a.applyBounds(adjustedPrice, originalPrice)

	return adjustedPrice
}

// AdjustBids adjusts all bids in a seat bid
func (a *Adjuster) AdjustBids(seatBid *openrtb.SeatBid, mediaType, publisherID string) {
	if !a.config.Enabled {
		return
	}

	bidderCode := seatBid.Seat
	for i := range seatBid.Bid {
		seatBid.Bid[i].Price = a.AdjustBid(&seatBid.Bid[i], bidderCode, mediaType, publisherID)
	}
}

// AdjustResponse adjusts all bids in a response
func (a *Adjuster) AdjustResponse(resp *openrtb.BidResponse, mediaTypes map[string]string, publisherID string) {
	if !a.config.Enabled || resp == nil {
		return
	}

	for i := range resp.SeatBid {
		bidderCode := resp.SeatBid[i].Seat
		for j := range resp.SeatBid[i].Bid {
			bid := &resp.SeatBid[i].Bid[j]
			mediaType := ""
			if mediaTypes != nil {
				mediaType = mediaTypes[bid.ImpID]
			}
			bid.Price = a.AdjustBid(bid, bidderCode, mediaType, publisherID)
		}
	}
}

// ruleMatches checks if a rule matches the given criteria
func (a *Adjuster) ruleMatches(rule *Rule, bidder, mediaType, dealID, publisherID string) bool {
	if rule.Bidder != "" && rule.Bidder != bidder {
		return false
	}
	if rule.MediaType != "" && rule.MediaType != mediaType {
		return false
	}
	if rule.DealID != "" && rule.DealID != dealID {
		return false
	}
	if rule.PublisherID != "" && rule.PublisherID != publisherID {
		return false
	}
	return true
}

// applyAdjustment applies a single adjustment rule
func (a *Adjuster) applyAdjustment(price float64, rule *Rule) float64 {
	switch rule.Type {
	case AdjustmentMultiplier:
		return price * rule.Value
	case AdjustmentCPM:
		return price + rule.Value
	case AdjustmentStatic:
		return rule.Value
	default:
		return price
	}
}

// applyBounds ensures the adjusted price is within configured bounds
func (a *Adjuster) applyBounds(adjustedPrice, originalPrice float64) float64 {
	// Check negative
	if !a.config.AllowNegative && adjustedPrice < 0 {
		adjustedPrice = 0
	}

	// Apply max multiplier bound
	if a.config.MaxAdjustment > 0 && originalPrice > 0 {
		maxPrice := originalPrice * a.config.MaxAdjustment
		if adjustedPrice > maxPrice {
			adjustedPrice = maxPrice
		}
	}

	// Apply min multiplier bound
	if a.config.MinAdjustment > 0 && originalPrice > 0 {
		minPrice := originalPrice * a.config.MinAdjustment
		if adjustedPrice < minPrice {
			adjustedPrice = minPrice
		}
	}

	return adjustedPrice
}

// SetEnabled enables/disables bid adjustments
func (a *Adjuster) SetEnabled(enabled bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.Enabled = enabled
}

// GetConfig returns current configuration
func (a *Adjuster) GetConfig() *Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

// AdjustmentResult contains the result of an adjustment calculation
type AdjustmentResult struct {
	OriginalPrice float64 `json:"original_price"`
	AdjustedPrice float64 `json:"adjusted_price"`
	RulesApplied  int     `json:"rules_applied"`
	BidderCode    string  `json:"bidder_code"`
	MediaType     string  `json:"media_type"`
}

// CalculateAdjustment calculates adjustment without modifying the bid (for debugging)
func (a *Adjuster) CalculateAdjustment(price float64, bidderCode, mediaType, dealID, publisherID string) *AdjustmentResult {
	result := &AdjustmentResult{
		OriginalPrice: price,
		AdjustedPrice: price,
		BidderCode:    bidderCode,
		MediaType:     mediaType,
	}

	if !a.config.Enabled {
		return result
	}

	a.mu.RLock()
	rules := a.rules
	a.mu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		if !a.ruleMatches(&rule, bidderCode, mediaType, dealID, publisherID) {
			continue
		}

		result.AdjustedPrice = a.applyAdjustment(result.AdjustedPrice, &rule)
		result.RulesApplied++
	}

	result.AdjustedPrice = a.applyBounds(result.AdjustedPrice, price)

	return result
}
