package exchange

import (
	"context"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	"github.com/thenexusengine/tne_springwire/internal/fpd"
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
				Bid:     &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.50},
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
			{Bid: &openrtb.Bid{ID: "b1", ImpID: "imp1", Price: 1.50}, BidType: adapters.BidTypeBanner},
			{Bid: &openrtb.Bid{ID: "b2", ImpID: "imp2", Price: 2.00}, BidType: adapters.BidTypeBanner},
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
		registry.Register("bidder"+string(rune('0'+i)), &mockAdapter{
			bids: []*adapters.TypedBid{
				{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: float64(i) * 0.5}, BidType: adapters.BidTypeBanner},
			},
		}, adapters.BidderInfo{Enabled: true})
	}

	ex := New(registry, &Config{
		DefaultTimeout:        100 * time.Millisecond,
		MaxConcurrentBidders:  10,
		IDREnabled:            false,
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
				{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.20}, BidType: adapters.BidTypeBanner},
				{Bid: &openrtb.Bid{ID: "bid2", ImpID: "imp2", Price: 2.50}, BidType: adapters.BidTypeBanner},
			},
		}, adapters.BidderInfo{Enabled: true})
	}

	ex := New(registry, &Config{
		DefaultTimeout:        150 * time.Millisecond,
		MaxConcurrentBidders:  10,
		IDREnabled:            false,
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

// ============================================================================
// BID VALIDATION BENCHMARKS
// ============================================================================

// BenchmarkValidateBid benchmarks bid validation
func BenchmarkValidateBid(b *testing.B) {
	ex := &Exchange{
		config: DefaultConfig(),
	}

	impFloorMap := map[string]float64{
		"imp1": 0.50,
		"imp2": 1.00,
	}

	bid := &openrtb.Bid{
		ID:    "bid1",
		ImpID: "imp1",
		Price: 1.50,
		AdM:   "<html>ad</html>",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ex.validateBid(bid, impFloorMap)
	}
}

// BenchmarkValidateBid_BelowFloor benchmarks bid rejection due to floor
func BenchmarkValidateBid_BelowFloor(b *testing.B) {
	ex := &Exchange{
		config: DefaultConfig(),
	}

	impFloorMap := map[string]float64{
		"imp1": 2.00,
	}

	bid := &openrtb.Bid{
		ID:    "bid1",
		ImpID: "imp1",
		Price: 1.50,
		AdM:   "<html>ad</html>",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ex.validateBid(bid, impFloorMap)
	}
}

// ============================================================================
// REQUEST CLONING BENCHMARKS
// ============================================================================

// BenchmarkCloneRequest_Simple benchmarks cloning a minimal request
func BenchmarkCloneRequest_Simple(b *testing.B) {
	ex := &Exchange{
		config: DefaultConfig(),
	}

	req := &openrtb.BidRequest{
		ID:   "test-req",
		Site: &openrtb.Site{ID: "site1", Domain: "example.com"},
		Imp: []openrtb.Imp{
			{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ex.cloneRequestWithFPD(req, "test_bidder", fpd.BidderFPD{})
	}
}

// BenchmarkCloneRequest_Complex benchmarks cloning a complex request with user data
func BenchmarkCloneRequest_Complex(b *testing.B) {
	ex := &Exchange{
		config: DefaultConfig(),
	}

	req := &openrtb.BidRequest{
		ID:   "test-req",
		Site: &openrtb.Site{
			ID:     "site1",
			Domain: "example.com",
			Page:   "https://example.com/page",
		},
		Device: &openrtb.Device{
			UA: "Mozilla/5.0...",
			IP: "192.168.1.1",
			Geo: &openrtb.Geo{
				Country: "US",
				Region:  "CA",
			},
		},
		User: &openrtb.User{
			ID: "user123",
			Ext: map[string]interface{}{
				"consent": "CPi8wgAPi8wgAAGABCENCZCsAP_AAH_AACiQHVNf_X_fb39j-_59_9t0eY1f9_7_v-0zjhfdt-8N2f_X_L8X42M7vF36pq4KuR4Eu3LBIQdlHOHcTUmw6IkVqTPsbk2Mr7NKJ7PEinMbe2dYGH9_n9XTuZKYr97s___z__-__v__75f_r-3_3_vp9X---_e_V399zLv9____39nP___9v-_9_____4IYgEmGpfAQJCQGwQAAAAA",
			},
		},
		Imp: []openrtb.Imp{
			{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
			{ID: "imp2", Banner: &openrtb.Banner{W: 728, H: 90}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ex.cloneRequestWithFPD(req, "test_bidder", fpd.BidderFPD{})
	}
}

// BenchmarkCloneRequest_WithFPD benchmarks cloning with FPD merging
func BenchmarkCloneRequest_WithFPD(b *testing.B) {
	ex := &Exchange{
		config: DefaultConfig(),
	}

	req := &openrtb.BidRequest{
		ID:   "test-req",
		Site: &openrtb.Site{ID: "site1", Domain: "example.com"},
		Imp: []openrtb.Imp{
			{ID: "imp1", Banner: &openrtb.Banner{W: 300, H: 250}},
		},
	}

	bidderFPD := fpd.BidderFPD{
		"test_bidder": fpd.FirstPartyData{
			Site: map[string]interface{}{
				"keywords": "sports,news",
			},
			User: map[string]interface{}{
				"keywords": "male,25-34",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ex.cloneRequestWithFPD(req, "test_bidder", bidderFPD)
	}
}

// ============================================================================
// BID DEDUPLICATION BENCHMARKS
// ============================================================================

// BenchmarkDeduplicateBids benchmarks bid deduplication
func BenchmarkDeduplicateBids(b *testing.B) {
	bids := []*adapters.TypedBid{
		{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.50}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.50}, BidType: adapters.BidTypeBanner}, // Duplicate
		{Bid: &openrtb.Bid{ID: "bid2", ImpID: "imp1", Price: 2.00}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid3", ImpID: "imp2", Price: 1.00}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 1.50}, BidType: adapters.BidTypeBanner}, // Another duplicate
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = deduplicateBids(bids)
	}
}

// ============================================================================
// AUCTION LOGIC BENCHMARKS
// ============================================================================

// BenchmarkSortBidsByPrice benchmarks bid sorting
func BenchmarkSortBidsByPrice(b *testing.B) {
	bids := []*adapters.TypedBid{
		{Bid: &openrtb.Bid{ID: "bid1", Price: 1.50}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid2", Price: 2.00}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid3", Price: 0.75}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid4", Price: 3.25}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid5", Price: 1.10}, BidType: adapters.BidTypeBanner},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sortBidsByPrice(bids)
	}
}

// BenchmarkSecondPriceAuction benchmarks second-price auction logic
func BenchmarkSecondPriceAuction(b *testing.B) {
	bids := []*adapters.TypedBid{
		{Bid: &openrtb.Bid{ID: "bid1", Price: 3.00}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid2", Price: 2.50}, BidType: adapters.BidTypeBanner},
		{Bid: &openrtb.Bid{ID: "bid3", Price: 2.00}, BidType: adapters.BidTypeBanner},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calculateSecondPriceClearingPrice(bids, 0.50, 0.01)
	}
}

// BenchmarkBuildImpFloorMap benchmarks floor map construction
func BenchmarkBuildImpFloorMap(b *testing.B) {
	req := &openrtb.BidRequest{
		Imp: []openrtb.Imp{
			{ID: "imp1", BidFloor: 0.50},
			{ID: "imp2", BidFloor: 1.00},
			{ID: "imp3", BidFloor: 2.00},
			{ID: "imp4", BidFloor: 0.75},
			{ID: "imp5", BidFloor: 1.50},
		},
	}

	publisherFloors := map[string]float64{
		"imp1": 0.60,
		"imp3": 2.50,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildImpFloorMap(req, publisherFloors)
	}
}

// ============================================================================
// PRICE BUCKETING BENCHMARKS
// ============================================================================

// BenchmarkFormatPriceBucket benchmarks price bucket calculation
func BenchmarkFormatPriceBucket(b *testing.B) {
	prices := []float64{0.50, 1.23, 5.67, 10.99, 25.00}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, price := range prices {
			_ = formatPriceBucket(price)
		}
	}
}

// ============================================================================
// BID MULTIPLIER BENCHMARKS
// ============================================================================

// BenchmarkApplyBidMultiplier benchmarks bid multiplier application
func BenchmarkApplyBidMultiplier(b *testing.B) {
	ex := &Exchange{
		config: DefaultConfig(),
	}

	ctx := context.Background()
	bidsByImp := map[string][]*adapters.TypedBid{
		"imp1": {
			{Bid: &openrtb.Bid{ID: "bid1", ImpID: "imp1", Price: 2.00, Ext: map[string]interface{}{
				"bid_multiplier": 1.2,
				"publisher_id":   "pub123",
			}}, BidType: adapters.BidTypeBanner},
			{Bid: &openrtb.Bid{ID: "bid2", ImpID: "imp1", Price: 1.50, Ext: map[string]interface{}{
				"bid_multiplier": 1.1,
				"publisher_id":   "pub123",
			}}, BidType: adapters.BidTypeBanner},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ex.applyBidMultiplier(ctx, bidsByImp)
	}
}
