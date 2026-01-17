package exchange

import (
	"context"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// ============================================================================
// AUCTION HOT PATH BENCHMARKS
// ============================================================================

// BenchmarkRunAuction_SingleImpression benchmarks a basic auction with 1 impression
func BenchmarkRunAuction_SingleImpression(b *testing.B) {
	registry := adapters.NewRegistry()
	registry.Register("test_bidder", &mockAdapter{
		bids: []*adapters.TypedBid{
			{
				Bid:     &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.50, AdM: "<html>ad</html>"},
				BidType: adapters.BidTypeBanner,
			},
		},
	}, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout: 100 * time.Millisecond,
		IDREnabled:     false,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "bench-req",
			Site: &openrtb.Site{ID: "site1", Domain: "example.com"},
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}, BidFloor: 0.50},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ex.RunAuction(context.Background(), req)
	}
}

// BenchmarkRunAuction_MultipleImpressions benchmarks auction with 5 impressions
func BenchmarkRunAuction_MultipleImpressions(b *testing.B) {
	registry := adapters.NewRegistry()
	registry.Register("bidder1", &mockAdapter{
		bids: []*adapters.TypedBid{
			{Bid: &openrtb.Bid{ID: "b1", ImpID: "imp1", Price: 1.50, AdM: "<html>ad</html>"}, BidType: adapters.BidTypeBanner},
			{Bid: &openrtb.Bid{ID: "b2", ImpID: "imp2", Price: 2.00, AdM: "<html>ad</html>"}, BidType: adapters.BidTypeBanner},
		},
	}, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout: 100 * time.Millisecond,
		IDREnabled:     false,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "bench-req",
			Site: &openrtb.Site{ID: "site1", Domain: "example.com"},
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
				{ID: "imp2", Banner: &openrtb.Banner{W: 728, H: 90}},
				{ID: "imp3", Banner: &openrtb.Banner{W: 160, H: 600}},
				{ID: "imp4", Banner: &openrtb.Banner{W: 320, H: 50}},
				{ID: "imp5", Banner: &openrtb.Banner{W: 970, H: 250}},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ex.RunAuction(context.Background(), req)
	}
}

// BenchmarkRunAuction_MultipleBidders benchmarks auction with 5 bidders
func BenchmarkRunAuction_MultipleBidders(b *testing.B) {
	registry := adapters.NewRegistry()

	for i := 1; i <= 5; i++ {
		bidderName := "bidder_" + string(rune('0'+i))
		registry.Register(bidderName, &mockAdapter{
			bids: []*adapters.TypedBid{
				{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: float64(i) * 0.5, AdM: "<html>ad</html>"}, BidType: adapters.BidTypeBanner},
			},
		}, adapters.BidderInfo{Enabled: true})
	}

	ex := New(registry, &Config{
		DefaultTimeout:       100 * time.Millisecond,
		MaxConcurrentBidders: 10,
		IDREnabled:           false,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "bench-req",
			Site: &openrtb.Site{ID: "site1", Domain: "example.com"},
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ex.RunAuction(context.Background(), req)
	}
}

// BenchmarkRunAuction_Realistic benchmarks a realistic scenario: 3 imps, 8 bidders
func BenchmarkRunAuction_Realistic(b *testing.B) {
	registry := adapters.NewRegistry()

	bidders := []string{"rubicon", "appnexus", "pubmatic", "openx", "ix", "sovrn", "smartadserver", "mediagrid"}
	for _, bidder := range bidders {
		registry.Register(bidder, &mockAdapter{
			bids: []*adapters.TypedBid{
				{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.20, AdM: "<html>ad</html>"}, BidType: adapters.BidTypeBanner},
				{Bid: &openrtb.Bid{ID: "bid2", ImpID: "imp2", Price: 2.50, AdM: "<html>ad</html>"}, BidType: adapters.BidTypeBanner},
			},
		}, adapters.BidderInfo{Enabled: true})
	}

	ex := New(registry, &Config{
		DefaultTimeout:       150 * time.Millisecond,
		MaxConcurrentBidders: 10,
		IDREnabled:           false,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "bench-req",
			Site: &openrtb.Site{ID: "site1", Domain: "example.com"},
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}, BidFloor: 0.50},
				{ID: "imp2", Banner: &openrtb.Banner{W: 728, H: 90}, BidFloor: 1.00},
				{ID: "imp3", Video: &openrtb.Video{W: 640, H: 480}, BidFloor: 2.00},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ex.RunAuction(context.Background(), req)
	}
}

// BenchmarkRunAuction_WithCircuitBreaker benchmarks auction with circuit breakers active
func BenchmarkRunAuction_WithCircuitBreaker(b *testing.B) {
	registry := adapters.NewRegistry()

	// Add some bidders
	registry.Register("bidder1", &mockAdapter{
		bids: []*adapters.TypedBid{
			{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.50, AdM: "<html>ad</html>"}, BidType: adapters.BidTypeBanner},
		},
	}, adapters.BidderInfo{Enabled: true})

	registry.Register("bidder2", &mockAdapter{
		bids: []*adapters.TypedBid{
			{Bid: &openrtb.Bid{ID: "bid2", ImpID: "imp1", Price: 2.00, AdM: "<html>ad</html>"}, BidType: adapters.BidTypeBanner},
		},
	}, adapters.BidderInfo{Enabled: true})

	ex := New(registry, &Config{
		DefaultTimeout: 100 * time.Millisecond,
		IDREnabled:     false,
	})

	req := &AuctionRequest{
		BidRequest: &openrtb.BidRequest{
			ID:   "bench-req",
			Site: &openrtb.Site{ID: "site1", Domain: "example.com"},
			Imp: []openrtb.Imp{
				{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ex.RunAuction(context.Background(), req)
	}
}

// BenchmarkGetBidderCircuitBreakerStats benchmarks stats retrieval
func BenchmarkGetBidderCircuitBreakerStats(b *testing.B) {
	registry := adapters.NewRegistry()

	for i := 1; i <= 10; i++ {
		registry.Register("bidder"+string(rune('0'+i)), &mockAdapter{}, adapters.BidderInfo{Enabled: true})
	}

	ex := New(registry, DefaultConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ex.GetBidderCircuitBreakerStats()
	}
}

