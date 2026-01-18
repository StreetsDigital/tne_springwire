# Business Metrics - TODO

The **Business Metrics** dashboard is ready but requires metric recording to be wired up in the exchange code.

## Metrics Defined But Not Yet Recorded

These metrics are defined in `internal/metrics/prometheus.go` but not being recorded:

### Auction Metrics
- `pbs_auctions_total{status, media_type}` - Total auctions by status
- `pbs_auction_duration_seconds{media_type}` - Auction duration histogram
- `pbs_bids_received_total{bidder, media_type}` - Bids received per bidder
- `pbs_bid_cpm{bidder, media_type}` - CPM distribution
- `pbs_bidders_selected{media_type}` - Number of bidders per auction
- `pbs_bidders_excluded{reason}` - Bidders excluded from auction

### Revenue Metrics
- `pbs_revenue_total{publisher, bidder, media_type}` - Total bid value
- `pbs_publisher_payout_total{publisher, bidder, media_type}` - Publisher payout
- `pbs_platform_margin_total{publisher, bidder, media_type}` - Platform margin
- `pbs_margin_percentage{publisher, bidder}` - Margin % distribution
- `pbs_floor_adjustments_total{publisher}` - Floor price adjustments

## Where to Add Recording

### In `internal/exchange/exchange.go`

#### 1. Record Auction Start/End
```go
// In HoldAuction() or runAuction()
startTime := time.Now()

// Record auction
m.AuctionsTotal.WithLabelValues(status, mediaType).Inc()
m.AuctionDuration.WithLabelValues(mediaType).Observe(time.Since(startTime).Seconds())
```

#### 2. Record Bids Received
```go
// When processing bidder responses
for _, bid := range bidderResponse.Bids {
    m.BidsReceived.WithLabelValues(bidderCode, mediaType).Inc()
    m.BidCPM.WithLabelValues(bidderCode, mediaType).Observe(bid.Bid.Price)
}
```

#### 3. Record Revenue (Already Partially There)
```go
// The RecordMargin() method is already called but needs to record to Prometheus
func (m *Metrics) RecordMargin(publisher, bidder, mediaType string, originalPrice, adjustedPrice, platformCut float64) {
    m.RevenueTotal.WithLabelValues(publisher, bidder, mediaType).Add(originalPrice)
    m.PublisherPayoutTotal.WithLabelValues(publisher, bidder, mediaType).Add(adjustedPrice)
    m.PlatformMarginTotal.WithLabelValues(publisher, bidder, mediaType).Add(platformCut)

    marginPct := platformCut / originalPrice
    m.MarginPercentage.WithLabelValues(publisher, bidder).Observe(marginPct)
}
```

## Labels to Extract

### Publisher Information
Extract from `openrtb.BidRequest`:
- Publisher ID: `req.Site.Publisher.ID` or `req.App.Publisher.ID`
- Domain: `req.Site.Domain` or `req.App.Bundle`

### Media Type
Determine from impression:
- `banner` if `imp.Banner != nil`
- `video` if `imp.Video != nil`
- `native` if `imp.Native != nil`
- `audio` if `imp.Audio != nil`

### Auction Status
- `success` - At least one bid received
- `no_bids` - No bids received
- `error` - Auction failed

## Alternative: Use Existing /admin/metrics

Alternatively, you could consume the existing `/admin/metrics` endpoint which tracks:
- `total_auctions`
- `successful_auctions`
- `failed_auctions`
- `total_bids`
- `bidder_stats`

This data could be scraped separately or exposed as Prometheus metrics.

## Quick Win

To get the dashboard working immediately, you could:

1. Add a simple exporter that reads `/admin/metrics` and converts to Prometheus format
2. Or wire up basic counters in the exchange for the top-level metrics

The dashboard structure is ready - just needs the data pipeline!
