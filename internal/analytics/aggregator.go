package analytics

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// Aggregator runs periodic aggregation jobs for analytics data.
// It calls PostgreSQL functions to roll up bid_events into
// hourly_stats and daily_publisher_stats tables.
type Aggregator struct {
	db     *sql.DB
	config *AggregatorConfig

	mu      sync.Mutex
	running bool
	done    chan struct{}

	// Stats
	lastHourlyRun  time.Time
	lastDailyRun   time.Time
	hourlyRunCount int64
	dailyRunCount  int64
	lastError      error
}

// AggregatorConfig holds aggregator configuration
type AggregatorConfig struct {
	// Enabled controls whether the aggregator runs
	Enabled bool `json:"enabled"`

	// HourlyInterval is how often to run hourly aggregation
	// Should be slightly after the hour to ensure data is complete
	HourlyInterval time.Duration `json:"hourly_interval"`

	// HourlyOffset is minutes after the hour to run (e.g., 5 = run at :05)
	HourlyOffset int `json:"hourly_offset"`

	// DailyHour is the hour (0-23) to run daily aggregation
	DailyHour int `json:"daily_hour"`

	// DailyMinute is the minute to run daily aggregation
	DailyMinute int `json:"daily_minute"`

	// QueryTimeout for aggregation queries
	QueryTimeout time.Duration `json:"query_timeout"`

	// RetentionMonths for partition cleanup (0 = no cleanup)
	RetentionMonths int `json:"retention_months"`
}

// DefaultAggregatorConfig returns sensible defaults
func DefaultAggregatorConfig() *AggregatorConfig {
	return &AggregatorConfig{
		Enabled:         true,
		HourlyInterval:  1 * time.Hour,
		HourlyOffset:    5, // Run at :05 past the hour
		DailyHour:       2, // Run at 02:30 AM
		DailyMinute:     30,
		QueryTimeout:    5 * time.Minute,
		RetentionMonths: 3, // Keep 3 months of data
	}
}

// NewAggregator creates a new analytics aggregator
func NewAggregator(db *sql.DB, config *AggregatorConfig) *Aggregator {
	if config == nil {
		config = DefaultAggregatorConfig()
	}

	return &Aggregator{
		db:     db,
		config: config,
		done:   make(chan struct{}),
	}
}

// Start begins the aggregation scheduler
func (a *Aggregator) Start() {
	a.mu.Lock()
	if a.running || !a.config.Enabled {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()

	logger.Log.Info().
		Int("hourly_offset", a.config.HourlyOffset).
		Int("daily_hour", a.config.DailyHour).
		Int("daily_minute", a.config.DailyMinute).
		Int("retention_months", a.config.RetentionMonths).
		Msg("Starting analytics aggregator")

	go a.runScheduler()
}

// Stop halts the aggregation scheduler
func (a *Aggregator) Stop() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	a.running = false
	a.mu.Unlock()

	close(a.done)
	logger.Log.Info().Msg("Stopped analytics aggregator")
}

// runScheduler is the main scheduling loop
func (a *Aggregator) runScheduler() {
	// Calculate time until next hourly run
	hourlyTicker := time.NewTicker(1 * time.Minute) // Check every minute
	defer hourlyTicker.Stop()

	for {
		select {
		case <-hourlyTicker.C:
			now := time.Now()

			// Check if it's time for hourly aggregation
			if now.Minute() == a.config.HourlyOffset {
				// Only run if we haven't run this hour
				hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
				if a.lastHourlyRun.Before(hourStart) {
					go a.runHourlyAggregation()
				}
			}

			// Check if it's time for daily aggregation
			if now.Hour() == a.config.DailyHour && now.Minute() == a.config.DailyMinute {
				// Only run if we haven't run today
				dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
				if a.lastDailyRun.Before(dayStart) {
					go a.runDailyAggregation()
				}
			}

		case <-a.done:
			return
		}
	}
}

// runHourlyAggregation executes the hourly aggregation function
func (a *Aggregator) runHourlyAggregation() {
	ctx, cancel := context.WithTimeout(context.Background(), a.config.QueryTimeout)
	defer cancel()

	start := time.Now()
	logger.Log.Debug().Msg("Running hourly analytics aggregation")

	// Call the PostgreSQL function
	var rowsAffected int
	err := a.db.QueryRowContext(ctx, "SELECT aggregate_hourly_stats()").Scan(&rowsAffected)

	a.mu.Lock()
	a.lastHourlyRun = time.Now()
	a.hourlyRunCount++
	a.lastError = err
	a.mu.Unlock()

	if err != nil {
		logger.Log.Error().
			Err(err).
			Dur("duration", time.Since(start)).
			Msg("Hourly aggregation failed")
		return
	}

	logger.Log.Info().
		Int("rows_affected", rowsAffected).
		Dur("duration", time.Since(start)).
		Msg("Hourly aggregation completed")
}

// runDailyAggregation executes daily tasks
func (a *Aggregator) runDailyAggregation() {
	ctx, cancel := context.WithTimeout(context.Background(), a.config.QueryTimeout)
	defer cancel()

	start := time.Now()
	logger.Log.Debug().Msg("Running daily analytics aggregation")

	var errs []error

	// 1. Run daily publisher stats
	var pubRows int
	err := a.db.QueryRowContext(ctx, "SELECT aggregate_daily_publisher_stats()").Scan(&pubRows)
	if err != nil {
		errs = append(errs, err)
		logger.Log.Error().Err(err).Msg("Daily publisher aggregation failed")
	} else {
		logger.Log.Info().Int("rows", pubRows).Msg("Daily publisher stats aggregated")
	}

	// 2. Create next month's partition (proactive)
	var partitionResult string
	err = a.db.QueryRowContext(ctx, "SELECT create_monthly_partition()").Scan(&partitionResult)
	if err != nil {
		errs = append(errs, err)
		logger.Log.Error().Err(err).Msg("Partition creation failed")
	} else {
		logger.Log.Info().Str("result", partitionResult).Msg("Partition check completed")
	}

	// 3. Drop old partitions if retention is configured
	if a.config.RetentionMonths > 0 {
		var dropResult string
		err = a.db.QueryRowContext(ctx,
			"SELECT drop_old_partitions($1)", a.config.RetentionMonths,
		).Scan(&dropResult)
		if err != nil {
			errs = append(errs, err)
			logger.Log.Error().Err(err).Msg("Partition cleanup failed")
		} else {
			logger.Log.Info().Str("result", dropResult).Msg("Partition cleanup completed")
		}
	}

	a.mu.Lock()
	a.lastDailyRun = time.Now()
	a.dailyRunCount++
	if len(errs) > 0 {
		a.lastError = errs[0]
	}
	a.mu.Unlock()

	logger.Log.Info().
		Dur("duration", time.Since(start)).
		Int("errors", len(errs)).
		Msg("Daily aggregation completed")
}

// RunNow executes aggregation immediately (for manual triggers)
func (a *Aggregator) RunNow(hourly, daily bool) error {
	if hourly {
		a.runHourlyAggregation()
	}
	if daily {
		a.runDailyAggregation()
	}

	a.mu.Lock()
	err := a.lastError
	a.mu.Unlock()

	return err
}

// BackfillHourlyStats aggregates historical data for a time range
func (a *Aggregator) BackfillHourlyStats(ctx context.Context, from, to time.Time) error {
	logger.Log.Info().
		Time("from", from).
		Time("to", to).
		Msg("Starting hourly stats backfill")

	current := from.Truncate(time.Hour)
	end := to.Truncate(time.Hour)

	count := 0
	for current.Before(end) || current.Equal(end) {
		var rows int
		err := a.db.QueryRowContext(ctx,
			"SELECT aggregate_hourly_stats($1)", current,
		).Scan(&rows)

		if err != nil {
			logger.Log.Error().
				Err(err).
				Time("hour", current).
				Msg("Backfill failed for hour")
			return err
		}

		count++
		current = current.Add(time.Hour)
	}

	logger.Log.Info().
		Int("hours_processed", count).
		Msg("Hourly stats backfill completed")

	return nil
}

// GetStats returns aggregator statistics
func (a *Aggregator) GetStats() AggregatorStats {
	a.mu.Lock()
	defer a.mu.Unlock()

	return AggregatorStats{
		Running:        a.running,
		LastHourlyRun:  a.lastHourlyRun,
		LastDailyRun:   a.lastDailyRun,
		HourlyRunCount: a.hourlyRunCount,
		DailyRunCount:  a.dailyRunCount,
		LastError:      a.lastError,
	}
}

// AggregatorStats holds runtime statistics
type AggregatorStats struct {
	Running        bool
	LastHourlyRun  time.Time
	LastDailyRun   time.Time
	HourlyRunCount int64
	DailyRunCount  int64
	LastError      error
}
