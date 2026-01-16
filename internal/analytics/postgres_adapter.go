package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// PostgresAdapter writes analytics events to PostgreSQL tables.
// It batches events for efficient bulk inserts and supports both
// the raw auction_events table and the denormalized bid_events table.
type PostgresAdapter struct {
	db            *sql.DB
	config        *PostgresAdapterConfig

	mu            sync.Mutex
	eventBatch    []*AuctionEvent
	bidBatch      []*bidEventRow
	done          chan struct{}
	closed        bool
}

// PostgresAdapterConfig holds configuration for the PostgreSQL adapter
type PostgresAdapterConfig struct {
	// BatchSize is the number of events to batch before inserting
	BatchSize int `json:"batch_size"`

	// FlushInterval is the maximum time to wait before flushing a batch
	FlushInterval time.Duration `json:"flush_interval"`

	// WriteRawEvents controls whether to write to auction_events table
	WriteRawEvents bool `json:"write_raw_events"`

	// WriteBidEvents controls whether to write to bid_events table
	WriteBidEvents bool `json:"write_bid_events"`

	// QueryTimeout for database operations
	QueryTimeout time.Duration `json:"query_timeout"`
}

// DefaultPostgresAdapterConfig returns sensible defaults
func DefaultPostgresAdapterConfig() *PostgresAdapterConfig {
	return &PostgresAdapterConfig{
		BatchSize:      100,
		FlushInterval:  5 * time.Second,
		WriteRawEvents: true,
		WriteBidEvents: true,
		QueryTimeout:   10 * time.Second,
	}
}

// bidEventRow is the flattened structure for bid_events table
type bidEventRow struct {
	Timestamp    time.Time
	RequestID    string
	PublisherID  string
	Domain       string
	BidderCode   string
	ImpID        string
	BidID        string
	DealID       string
	MediaType    string
	BidPrice     float64
	BidCurrency  string
	FloorPrice   float64
	IsWinner     bool
	IsTimeout    bool
	IsError      bool
	IsNoBid      bool
	ErrorCode    string
	LatencyMs    int
}

// NewPostgresAdapter creates a new PostgreSQL analytics adapter
func NewPostgresAdapter(db *sql.DB, config *PostgresAdapterConfig) *PostgresAdapter {
	if config == nil {
		config = DefaultPostgresAdapterConfig()
	}

	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}
	if config.QueryTimeout <= 0 {
		config.QueryTimeout = 10 * time.Second
	}

	a := &PostgresAdapter{
		db:         db,
		config:     config,
		eventBatch: make([]*AuctionEvent, 0, config.BatchSize),
		bidBatch:   make([]*bidEventRow, 0, config.BatchSize),
		done:       make(chan struct{}),
	}

	// Start background flush goroutine
	go a.flushLoop()

	return a
}

// Name returns the adapter name
func (a *PostgresAdapter) Name() string {
	return "postgres"
}

// LogAuctionEvent queues an event for batch insertion
func (a *PostgresAdapter) LogAuctionEvent(ctx context.Context, event *AuctionEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}

	// Add to raw events batch if enabled
	if a.config.WriteRawEvents {
		a.eventBatch = append(a.eventBatch, event)
	}

	// Convert bid-related events to bid_events rows
	if a.config.WriteBidEvents {
		if row := a.eventToBidRow(event); row != nil {
			a.bidBatch = append(a.bidBatch, row)
		}
	}

	// Check if we should flush
	shouldFlush := len(a.eventBatch) >= a.config.BatchSize ||
	               len(a.bidBatch) >= a.config.BatchSize

	if shouldFlush {
		// Flush in background to avoid blocking
		eventBatch := a.eventBatch
		bidBatch := a.bidBatch
		a.eventBatch = make([]*AuctionEvent, 0, a.config.BatchSize)
		a.bidBatch = make([]*bidEventRow, 0, a.config.BatchSize)

		go a.flushBatches(eventBatch, bidBatch)
	}

	return nil
}

// eventToBidRow converts bid-related events to bid_events rows
func (a *PostgresAdapter) eventToBidRow(event *AuctionEvent) *bidEventRow {
	switch event.Type {
	case EventBidResponse:
		return &bidEventRow{
			Timestamp:   event.Timestamp,
			RequestID:   event.RequestID,
			PublisherID: event.PublisherID,
			Domain:      event.Domain,
			BidderCode:  event.BidderCode,
			ImpID:       event.ImpID,
			BidID:       event.BidID,
			DealID:      event.DealID,
			BidPrice:    event.BidPrice,
			BidCurrency: coalesce(event.BidCurrency, "USD"),
			LatencyMs:   int(event.Duration.Milliseconds()),
		}

	case EventBidWon:
		return &bidEventRow{
			Timestamp:   event.Timestamp,
			RequestID:   event.RequestID,
			PublisherID: event.PublisherID,
			Domain:      event.Domain,
			BidderCode:  event.BidderCode,
			ImpID:       event.ImpID,
			BidID:       event.BidID,
			BidPrice:    event.BidPrice,
			BidCurrency: coalesce(event.BidCurrency, "USD"),
			IsWinner:    true,
		}

	case EventNoBid:
		return &bidEventRow{
			Timestamp:   event.Timestamp,
			RequestID:   event.RequestID,
			PublisherID: event.PublisherID,
			Domain:      event.Domain,
			BidderCode:  event.BidderCode,
			IsNoBid:     true,
		}

	case EventBidTimeout:
		return &bidEventRow{
			Timestamp:   event.Timestamp,
			RequestID:   event.RequestID,
			PublisherID: event.PublisherID,
			Domain:      event.Domain,
			BidderCode:  event.BidderCode,
			IsTimeout:   true,
			LatencyMs:   int(event.Duration.Milliseconds()),
		}

	case EventBidError:
		return &bidEventRow{
			Timestamp:   event.Timestamp,
			RequestID:   event.RequestID,
			PublisherID: event.PublisherID,
			Domain:      event.Domain,
			BidderCode:  event.BidderCode,
			IsError:     true,
			ErrorCode:   event.ErrorCode,
		}

	default:
		return nil
	}
}

// flushLoop runs the periodic flush
func (a *PostgresAdapter) flushLoop() {
	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.flush()
		case <-a.done:
			return
		}
	}
}

// flush grabs current batches and writes them
func (a *PostgresAdapter) flush() {
	a.mu.Lock()
	if len(a.eventBatch) == 0 && len(a.bidBatch) == 0 {
		a.mu.Unlock()
		return
	}

	eventBatch := a.eventBatch
	bidBatch := a.bidBatch
	a.eventBatch = make([]*AuctionEvent, 0, a.config.BatchSize)
	a.bidBatch = make([]*bidEventRow, 0, a.config.BatchSize)
	a.mu.Unlock()

	a.flushBatches(eventBatch, bidBatch)
}

// flushBatches writes both batches to the database
func (a *PostgresAdapter) flushBatches(events []*AuctionEvent, bids []*bidEventRow) {
	ctx, cancel := context.WithTimeout(context.Background(), a.config.QueryTimeout)
	defer cancel()

	// Write raw events
	if len(events) > 0 && a.config.WriteRawEvents {
		if err := a.insertAuctionEvents(ctx, events); err != nil {
			logger.Log.Debug().
				Err(err).
				Int("count", len(events)).
				Msg("Failed to insert auction_events batch")
		}
	}

	// Write bid events
	if len(bids) > 0 && a.config.WriteBidEvents {
		if err := a.insertBidEvents(ctx, bids); err != nil {
			logger.Log.Debug().
				Err(err).
				Int("count", len(bids)).
				Msg("Failed to insert bid_events batch")
		}
	}
}

// insertAuctionEvents bulk inserts into auction_events table
func (a *PostgresAdapter) insertAuctionEvents(ctx context.Context, events []*AuctionEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Build multi-row INSERT
	valueStrings := make([]string, 0, len(events))
	valueArgs := make([]interface{}, 0, len(events)*13)

	for i, e := range events {
		base := i * 13
		valueStrings = append(valueStrings, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7,
			base+8, base+9, base+10, base+11, base+12, base+13,
		))

		// Convert extra to JSONB-compatible format
		var extraJSON interface{} = nil
		if len(e.Extra) > 0 {
			extraJSON = e.Extra
		}

		valueArgs = append(valueArgs,
			string(e.Type),
			e.Timestamp,
			e.RequestID,
			nullString(e.PublisherID),
			nullString(e.Domain),
			nullString(e.AppBundle),
			nullString(e.BidderCode),
			nullString(e.ImpID),
			nullFloat64(e.BidPrice),
			nullInt(int(e.Duration.Milliseconds())),
			e.GDPRApplies,
			nullString(e.ErrorCode),
			extraJSON,
		)
	}

	query := fmt.Sprintf(`
		INSERT INTO auction_events (
			event_type, timestamp, request_id,
			publisher_id, domain, app_bundle, bidder_code,
			imp_id, bid_price, duration_ms, gdpr_applies,
			error_code, extra
		) VALUES %s
	`, strings.Join(valueStrings, ", "))

	_, err := a.db.ExecContext(ctx, query, valueArgs...)
	return err
}

// insertBidEvents bulk inserts into bid_events table
func (a *PostgresAdapter) insertBidEvents(ctx context.Context, bids []*bidEventRow) error {
	if len(bids) == 0 {
		return nil
	}

	// Build multi-row INSERT
	valueStrings := make([]string, 0, len(bids))
	valueArgs := make([]interface{}, 0, len(bids)*16)

	for i, b := range bids {
		base := i * 16
		valueStrings = append(valueStrings, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8,
			base+9, base+10, base+11, base+12, base+13, base+14, base+15, base+16,
		))

		valueArgs = append(valueArgs,
			b.Timestamp,
			b.RequestID,
			nullString(b.PublisherID),
			nullString(b.Domain),
			b.BidderCode,
			nullString(b.ImpID),
			nullString(b.BidID),
			nullString(b.DealID),
			nullString(b.MediaType),
			b.BidPrice,
			b.BidCurrency,
			b.IsWinner,
			b.IsTimeout,
			b.IsError,
			b.IsNoBid,
			nullInt(b.LatencyMs),
		)
	}

	query := fmt.Sprintf(`
		INSERT INTO bid_events (
			timestamp, request_id, publisher_id, domain,
			bidder_code, imp_id, bid_id, deal_id, media_type,
			bid_price, bid_currency,
			is_winner, is_timeout, is_error, is_no_bid,
			latency_ms
		) VALUES %s
	`, strings.Join(valueStrings, ", "))

	_, err := a.db.ExecContext(ctx, query, valueArgs...)
	return err
}

// Close flushes remaining events and stops the adapter
func (a *PostgresAdapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()

	close(a.done)

	// Final flush
	a.flush()

	return nil
}

// GetStats returns current batch sizes (for monitoring)
func (a *PostgresAdapter) GetStats() (eventBatchSize, bidBatchSize int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.eventBatch), len(a.bidBatch)
}

// Helper functions

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullFloat64(f float64) interface{} {
	if f == 0 {
		return nil
	}
	return f
}

func nullInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

func coalesce(s, defaultVal string) string {
	if s == "" {
		return defaultVal
	}
	return s
}
