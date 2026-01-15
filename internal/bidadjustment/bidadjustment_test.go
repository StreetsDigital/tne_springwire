package bidadjustment

import (
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

func TestAdjuster_Multiplier(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		Bidder:  "appnexus",
		Type:    AdjustmentMultiplier,
		Value:   0.9, // 10% reduction
		Enabled: true,
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "appnexus", "banner", "")

	if adjusted != 0.9 {
		t.Errorf("expected 0.9, got %f", adjusted)
	}
}

func TestAdjuster_CPM(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		Bidder:  "rubicon",
		Type:    AdjustmentCPM,
		Value:   0.25, // Add $0.25
		Enabled: true,
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "rubicon", "video", "")

	if adjusted != 1.25 {
		t.Errorf("expected 1.25, got %f", adjusted)
	}
}

func TestAdjuster_Static(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		Bidder:  "test",
		Type:    AdjustmentStatic,
		Value:   5.00,
		Enabled: true,
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "test", "banner", "")

	// Static adjustment capped by MaxAdjustment (5x = 5.00)
	if adjusted != 5.00 {
		t.Errorf("expected 5.00, got %f", adjusted)
	}
}

func TestAdjuster_MediaTypeMatching(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		MediaType: "video",
		Type:      AdjustmentMultiplier,
		Value:     1.5,
		Enabled:   true,
	})

	// Video should match
	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "any", "video", "")
	if adjusted != 1.50 {
		t.Errorf("expected 1.50 for video, got %f", adjusted)
	}

	// Banner should not match
	bid = &openrtb.Bid{Price: 1.00}
	adjusted = adjuster.AdjustBid(bid, "any", "banner", "")
	if adjusted != 1.00 {
		t.Errorf("expected 1.00 for banner (no match), got %f", adjusted)
	}
}

func TestAdjuster_DealMatching(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		DealID:  "deal-123",
		Type:    AdjustmentMultiplier,
		Value:   1.2,
		Enabled: true,
	})

	// Matching deal
	bid := &openrtb.Bid{Price: 1.00, DealID: "deal-123"}
	adjusted := adjuster.AdjustBid(bid, "any", "banner", "")
	if adjusted != 1.20 {
		t.Errorf("expected 1.20 for matching deal, got %f", adjusted)
	}

	// Non-matching deal
	bid = &openrtb.Bid{Price: 1.00, DealID: "deal-456"}
	adjusted = adjuster.AdjustBid(bid, "any", "banner", "")
	if adjusted != 1.00 {
		t.Errorf("expected 1.00 for non-matching deal, got %f", adjusted)
	}
}

func TestAdjuster_MultipleRules(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())

	// Add rules in different order but with priorities
	adjuster.AddRule(Rule{
		Bidder:   "appnexus",
		Type:     AdjustmentMultiplier,
		Value:    0.9,
		Priority: 1,
		Enabled:  true,
	})
	adjuster.AddRule(Rule{
		MediaType: "video",
		Type:      AdjustmentMultiplier,
		Value:     1.2,
		Priority:  2, // Higher priority, applied first
		Enabled:   true,
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "appnexus", "video", "")

	// Expected: 1.00 * 1.2 (video first) * 0.9 (appnexus second) = 1.08
	expected := 1.08
	diff := adjusted - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.001 {
		t.Errorf("expected %f, got %f", expected, adjusted)
	}
}

func TestAdjuster_MaxAdjustment(t *testing.T) {
	config := DefaultConfig()
	config.MaxAdjustment = 2.0 // Max 2x
	adjuster := NewAdjuster(config)

	adjuster.AddRule(Rule{
		Bidder:  "test",
		Type:    AdjustmentMultiplier,
		Value:   5.0, // 5x increase
		Enabled: true,
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "test", "banner", "")

	// Should be capped at 2x
	if adjusted != 2.00 {
		t.Errorf("expected 2.00 (max capped), got %f", adjusted)
	}
}

func TestAdjuster_MinAdjustment(t *testing.T) {
	config := DefaultConfig()
	config.MinAdjustment = 0.5 // Min 0.5x
	adjuster := NewAdjuster(config)

	adjuster.AddRule(Rule{
		Bidder:  "test",
		Type:    AdjustmentMultiplier,
		Value:   0.1, // 90% reduction
		Enabled: true,
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "test", "banner", "")

	// Should be floored at 0.5x
	if adjusted != 0.50 {
		t.Errorf("expected 0.50 (min floored), got %f", adjusted)
	}
}

func TestAdjuster_DisabledRule(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		Bidder:  "test",
		Type:    AdjustmentMultiplier,
		Value:   2.0,
		Enabled: false, // Disabled
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "test", "banner", "")

	// Rule should not apply
	if adjusted != 1.00 {
		t.Errorf("expected 1.00 (disabled rule), got %f", adjusted)
	}
}

func TestAdjuster_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	adjuster := NewAdjuster(config)

	adjuster.AddRule(Rule{
		Bidder:  "test",
		Type:    AdjustmentMultiplier,
		Value:   2.0,
		Enabled: true,
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "test", "banner", "")

	// Adjuster disabled, no adjustment
	if adjusted != 1.00 {
		t.Errorf("expected 1.00 (adjuster disabled), got %f", adjusted)
	}
}

func TestAdjuster_AdjustBids(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		Bidder:  "appnexus",
		Type:    AdjustmentMultiplier,
		Value:   0.9,
		Enabled: true,
	})

	seatBid := &openrtb.SeatBid{
		Seat: "appnexus",
		Bid: []openrtb.Bid{
			{ID: "1", Price: 1.00},
			{ID: "2", Price: 2.00},
		},
	}

	adjuster.AdjustBids(seatBid, "banner", "pub123")

	if seatBid.Bid[0].Price != 0.90 {
		t.Errorf("expected first bid 0.90, got %f", seatBid.Bid[0].Price)
	}
	if seatBid.Bid[1].Price != 1.80 {
		t.Errorf("expected second bid 1.80, got %f", seatBid.Bid[1].Price)
	}
}

func TestAdjuster_AdjustResponse(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		MediaType: "video",
		Type:      AdjustmentMultiplier,
		Value:     1.5,
		Enabled:   true,
	})

	resp := &openrtb.BidResponse{
		SeatBid: []openrtb.SeatBid{
			{
				Seat: "bidder1",
				Bid: []openrtb.Bid{
					{ID: "1", ImpID: "imp1", Price: 1.00},
					{ID: "2", ImpID: "imp2", Price: 2.00},
				},
			},
		},
	}

	mediaTypes := map[string]string{
		"imp1": "video",
		"imp2": "banner",
	}

	adjuster.AdjustResponse(resp, mediaTypes, "pub123")

	// Video imp should be adjusted
	if resp.SeatBid[0].Bid[0].Price != 1.50 {
		t.Errorf("expected video bid 1.50, got %f", resp.SeatBid[0].Bid[0].Price)
	}

	// Banner imp should not be adjusted
	if resp.SeatBid[0].Bid[1].Price != 2.00 {
		t.Errorf("expected banner bid 2.00, got %f", resp.SeatBid[0].Bid[1].Price)
	}
}

func TestAdjuster_SetRules(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())

	rules := []Rule{
		{Bidder: "b", Type: AdjustmentMultiplier, Value: 1.0, Priority: 1, Enabled: true},
		{Bidder: "a", Type: AdjustmentMultiplier, Value: 1.0, Priority: 3, Enabled: true},
		{Bidder: "c", Type: AdjustmentMultiplier, Value: 1.0, Priority: 2, Enabled: true},
	}

	adjuster.SetRules(rules)

	got := adjuster.GetRules()
	if len(got) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(got))
	}

	// Should be sorted by priority descending
	if got[0].Bidder != "a" || got[1].Bidder != "c" || got[2].Bidder != "b" {
		t.Error("rules not sorted by priority")
	}
}

func TestAdjuster_RemoveRule(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{Bidder: "a", MediaType: "banner", Enabled: true})
	adjuster.AddRule(Rule{Bidder: "b", MediaType: "video", Enabled: true})

	adjuster.RemoveRule("a", "banner")

	rules := adjuster.GetRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after removal, got %d", len(rules))
	}
	if rules[0].Bidder != "b" {
		t.Error("wrong rule removed")
	}
}

func TestAdjuster_ClearRules(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{Bidder: "a", Enabled: true})
	adjuster.AddRule(Rule{Bidder: "b", Enabled: true})

	adjuster.ClearRules()

	if len(adjuster.GetRules()) != 0 {
		t.Error("expected 0 rules after clear")
	}
}

func TestAdjuster_CalculateAdjustment(t *testing.T) {
	adjuster := NewAdjuster(DefaultConfig())
	adjuster.AddRule(Rule{
		Bidder:  "test",
		Type:    AdjustmentMultiplier,
		Value:   0.9,
		Enabled: true,
	})
	adjuster.AddRule(Rule{
		MediaType: "video",
		Type:      AdjustmentCPM,
		Value:     0.1,
		Enabled:   true,
	})

	result := adjuster.CalculateAdjustment(1.00, "test", "video", "", "")

	if result.OriginalPrice != 1.00 {
		t.Errorf("expected original price 1.00, got %f", result.OriginalPrice)
	}

	if result.RulesApplied != 2 {
		t.Errorf("expected 2 rules applied, got %d", result.RulesApplied)
	}

	// 1.00 * 0.9 + 0.1 = 1.0
	expected := 1.00
	diff := result.AdjustedPrice - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.001 {
		t.Errorf("expected adjusted price %f, got %f", expected, result.AdjustedPrice)
	}
}

func TestAdjuster_NegativeNotAllowed(t *testing.T) {
	config := DefaultConfig()
	config.AllowNegative = false
	adjuster := NewAdjuster(config)

	adjuster.AddRule(Rule{
		Bidder:  "test",
		Type:    AdjustmentCPM,
		Value:   -2.00, // Subtract $2
		Enabled: true,
	})

	bid := &openrtb.Bid{Price: 1.00}
	adjusted := adjuster.AdjustBid(bid, "test", "banner", "")

	// Should be floored at 0 (or min adjustment)
	if adjusted < 0 {
		t.Errorf("expected non-negative price, got %f", adjusted)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("expected enabled by default")
	}

	if config.MaxAdjustment != 5.0 {
		t.Errorf("expected max adjustment 5.0, got %f", config.MaxAdjustment)
	}

	if config.MinAdjustment != 0.1 {
		t.Errorf("expected min adjustment 0.1, got %f", config.MinAdjustment)
	}

	if config.AllowNegative {
		t.Error("expected AllowNegative false by default")
	}
}
