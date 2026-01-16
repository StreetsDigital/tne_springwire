-- =====================================================
-- Catalyst Analytics Schema
-- =====================================================
-- This migration creates tables for storing auction
-- analytics events, enabling revenue tracking, bidder
-- performance monitoring, and business intelligence.
--
-- Design decisions:
-- - Partitioned by day for efficient data retention
-- - Denormalized bid_events for fast revenue queries
-- - Pre-aggregated hourly_stats to reduce dashboard load
-- - JSONB extra field for flexible event metadata
-- =====================================================

-- =====================================================
-- Main auction events table (partitioned by day)
-- =====================================================
-- Stores all event types: auction_start, auction_end,
-- bid_request, bid_response, no_bid, bid_won, bid_timeout,
-- bid_error, cookie_sync, floor_enforced, privacy_filtered
-- =====================================================

CREATE TABLE IF NOT EXISTS auction_events (
    id              BIGSERIAL,
    event_type      VARCHAR(30) NOT NULL,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_id      VARCHAR(64) NOT NULL,

    -- Dimensions (for GROUP BY queries)
    publisher_id    VARCHAR(255),
    domain          VARCHAR(255),
    app_bundle      VARCHAR(255),
    bidder_code     VARCHAR(50),

    -- Impression context
    imp_id          VARCHAR(64),
    bid_id          VARCHAR(64),
    deal_id         VARCHAR(64),

    -- Metrics
    bid_price       DECIMAL(12,6),
    bid_currency    VARCHAR(3) DEFAULT 'USD',
    duration_ms     INTEGER,

    -- Flags
    gdpr_applies    BOOLEAN DEFAULT FALSE,

    -- Error context
    error_code      VARCHAR(50),
    error_message   TEXT,

    -- Flexible metadata (for extra fields)
    extra           JSONB DEFAULT '{}',

    -- Partition key must be in primary key
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);

-- Create default partition for any data outside defined ranges
CREATE TABLE IF NOT EXISTS auction_events_default
    PARTITION OF auction_events DEFAULT;

-- Create partition for current month
DO $$
DECLARE
    start_date DATE := DATE_TRUNC('month', CURRENT_DATE);
    end_date DATE := DATE_TRUNC('month', CURRENT_DATE) + INTERVAL '1 month';
    partition_name TEXT;
BEGIN
    partition_name := 'auction_events_' || TO_CHAR(start_date, 'YYYY_MM');

    IF NOT EXISTS (
        SELECT 1 FROM pg_class WHERE relname = partition_name
    ) THEN
        EXECUTE FORMAT(
            'CREATE TABLE %I PARTITION OF auction_events FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
    END IF;
END $$;

-- Create partition for next month (proactive)
DO $$
DECLARE
    start_date DATE := DATE_TRUNC('month', CURRENT_DATE) + INTERVAL '1 month';
    end_date DATE := DATE_TRUNC('month', CURRENT_DATE) + INTERVAL '2 months';
    partition_name TEXT;
BEGIN
    partition_name := 'auction_events_' || TO_CHAR(start_date, 'YYYY_MM');

    IF NOT EXISTS (
        SELECT 1 FROM pg_class WHERE relname = partition_name
    ) THEN
        EXECUTE FORMAT(
            'CREATE TABLE %I PARTITION OF auction_events FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
    END IF;
END $$;

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_auction_events_timestamp
    ON auction_events (timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_auction_events_publisher_time
    ON auction_events (publisher_id, timestamp DESC)
    WHERE publisher_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_auction_events_bidder_time
    ON auction_events (bidder_code, timestamp DESC)
    WHERE bidder_code IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_auction_events_type_time
    ON auction_events (event_type, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_auction_events_request
    ON auction_events (request_id, timestamp DESC);

-- GIN index for JSONB extra field queries
CREATE INDEX IF NOT EXISTS idx_auction_events_extra
    ON auction_events USING GIN (extra);

-- =====================================================
-- Denormalized bid events table (fast revenue queries)
-- =====================================================
-- Flattened view of bid-related events optimized for:
-- - Revenue reporting by publisher/bidder
-- - Win rate analysis
-- - Latency monitoring
-- - Error tracking
-- =====================================================

CREATE TABLE IF NOT EXISTS bid_events (
    id              BIGSERIAL PRIMARY KEY,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_id      VARCHAR(64) NOT NULL,

    -- Who
    publisher_id    VARCHAR(255),
    domain          VARCHAR(255),
    bidder_code     VARCHAR(50) NOT NULL,

    -- What
    imp_id          VARCHAR(64),
    bid_id          VARCHAR(64),
    deal_id         VARCHAR(64),
    media_type      VARCHAR(20),  -- 'banner', 'video', 'native', 'audio'

    -- Money
    bid_price       DECIMAL(12,6) NOT NULL DEFAULT 0,
    bid_currency    VARCHAR(3) DEFAULT 'USD',
    floor_price     DECIMAL(12,6),

    -- Outcome
    is_winner       BOOLEAN DEFAULT FALSE,
    is_timeout      BOOLEAN DEFAULT FALSE,
    is_error        BOOLEAN DEFAULT FALSE,
    is_no_bid       BOOLEAN DEFAULT FALSE,
    error_code      VARCHAR(50),

    -- Performance
    latency_ms      INTEGER,

    -- Timestamps
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for bid_events queries
CREATE INDEX IF NOT EXISTS idx_bid_events_timestamp
    ON bid_events (timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_bid_events_publisher_time
    ON bid_events (publisher_id, timestamp DESC)
    WHERE publisher_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bid_events_bidder_time
    ON bid_events (bidder_code, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_bid_events_winners
    ON bid_events (timestamp DESC)
    WHERE is_winner = TRUE;
CREATE INDEX IF NOT EXISTS idx_bid_events_errors
    ON bid_events (bidder_code, timestamp DESC)
    WHERE is_error = TRUE OR is_timeout = TRUE;

-- =====================================================
-- Pre-aggregated hourly statistics
-- =====================================================
-- Rolled up metrics for fast dashboard queries.
-- Updated by periodic aggregation job.
-- =====================================================

CREATE TABLE IF NOT EXISTS hourly_stats (
    hour            TIMESTAMPTZ NOT NULL,
    publisher_id    VARCHAR(255) NOT NULL DEFAULT '__all__',
    bidder_code     VARCHAR(50) NOT NULL DEFAULT '__all__',

    -- Request counts
    auction_count   BIGINT DEFAULT 0,
    bid_request_count BIGINT DEFAULT 0,

    -- Response counts
    bid_count       BIGINT DEFAULT 0,
    win_count       BIGINT DEFAULT 0,
    no_bid_count    BIGINT DEFAULT 0,
    timeout_count   BIGINT DEFAULT 0,
    error_count     BIGINT DEFAULT 0,

    -- Revenue metrics
    total_bid_value     DECIMAL(18,6) DEFAULT 0,
    winning_bid_value   DECIMAL(18,6) DEFAULT 0,
    avg_bid_price       DECIMAL(12,6) DEFAULT 0,
    max_bid_price       DECIMAL(12,6) DEFAULT 0,

    -- Latency metrics (milliseconds)
    avg_latency_ms  INTEGER DEFAULT 0,
    min_latency_ms  INTEGER DEFAULT 0,
    max_latency_ms  INTEGER DEFAULT 0,
    p50_latency_ms  INTEGER DEFAULT 0,
    p95_latency_ms  INTEGER DEFAULT 0,
    p99_latency_ms  INTEGER DEFAULT 0,

    -- Rates (pre-calculated for convenience)
    win_rate        DECIMAL(5,4) DEFAULT 0,  -- wins / bids
    fill_rate       DECIMAL(5,4) DEFAULT 0,  -- bids / requests
    error_rate      DECIMAL(5,4) DEFAULT 0,  -- errors / requests

    -- Metadata
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),

    PRIMARY KEY (hour, publisher_id, bidder_code)
);

-- Index for time-range queries on hourly_stats
CREATE INDEX IF NOT EXISTS idx_hourly_stats_hour
    ON hourly_stats (hour DESC);
CREATE INDEX IF NOT EXISTS idx_hourly_stats_publisher
    ON hourly_stats (publisher_id, hour DESC);
CREATE INDEX IF NOT EXISTS idx_hourly_stats_bidder
    ON hourly_stats (bidder_code, hour DESC);

-- =====================================================
-- Daily publisher summary (for business reporting)
-- =====================================================

CREATE TABLE IF NOT EXISTS daily_publisher_stats (
    date            DATE NOT NULL,
    publisher_id    VARCHAR(255) NOT NULL,

    -- Volume
    total_auctions      BIGINT DEFAULT 0,
    total_impressions   BIGINT DEFAULT 0,
    total_bids          BIGINT DEFAULT 0,
    total_wins          BIGINT DEFAULT 0,

    -- Revenue
    gross_revenue       DECIMAL(18,6) DEFAULT 0,  -- total winning bids
    estimated_rpm       DECIMAL(12,6) DEFAULT 0,  -- revenue per mille

    -- Performance
    avg_win_price       DECIMAL(12,6) DEFAULT 0,
    fill_rate           DECIMAL(5,4) DEFAULT 0,

    -- Top bidders (JSONB array)
    top_bidders         JSONB DEFAULT '[]',

    -- Metadata
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),

    PRIMARY KEY (date, publisher_id)
);

CREATE INDEX IF NOT EXISTS idx_daily_publisher_date
    ON daily_publisher_stats (date DESC);

-- =====================================================
-- Function: Aggregate hourly stats from bid_events
-- =====================================================
-- Call this function via pg_cron or application cron
-- to roll up the last hour's data into hourly_stats.
-- =====================================================

CREATE OR REPLACE FUNCTION aggregate_hourly_stats(target_hour TIMESTAMPTZ DEFAULT NULL)
RETURNS INTEGER AS $$
DECLARE
    hour_start TIMESTAMPTZ;
    hour_end TIMESTAMPTZ;
    rows_affected INTEGER := 0;
BEGIN
    -- Default to the previous complete hour
    IF target_hour IS NULL THEN
        hour_start := DATE_TRUNC('hour', NOW() - INTERVAL '1 hour');
    ELSE
        hour_start := DATE_TRUNC('hour', target_hour);
    END IF;
    hour_end := hour_start + INTERVAL '1 hour';

    -- Aggregate by publisher and bidder
    INSERT INTO hourly_stats (
        hour, publisher_id, bidder_code,
        bid_request_count, bid_count, win_count, no_bid_count,
        timeout_count, error_count,
        total_bid_value, winning_bid_value, avg_bid_price, max_bid_price,
        avg_latency_ms, min_latency_ms, max_latency_ms,
        p50_latency_ms, p95_latency_ms, p99_latency_ms,
        win_rate, fill_rate, error_rate
    )
    SELECT
        hour_start,
        COALESCE(publisher_id, '__unknown__'),
        bidder_code,
        COUNT(*),
        COUNT(*) FILTER (WHERE bid_price > 0 AND NOT is_no_bid),
        COUNT(*) FILTER (WHERE is_winner),
        COUNT(*) FILTER (WHERE is_no_bid),
        COUNT(*) FILTER (WHERE is_timeout),
        COUNT(*) FILTER (WHERE is_error),
        COALESCE(SUM(bid_price) FILTER (WHERE bid_price > 0), 0),
        COALESCE(SUM(bid_price) FILTER (WHERE is_winner), 0),
        COALESCE(AVG(bid_price) FILTER (WHERE bid_price > 0), 0),
        COALESCE(MAX(bid_price), 0),
        COALESCE(AVG(latency_ms)::INTEGER, 0),
        COALESCE(MIN(latency_ms), 0),
        COALESCE(MAX(latency_ms), 0),
        COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY latency_ms)::INTEGER, 0),
        COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms)::INTEGER, 0),
        COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY latency_ms)::INTEGER, 0),
        CASE
            WHEN COUNT(*) FILTER (WHERE bid_price > 0) > 0
            THEN COUNT(*) FILTER (WHERE is_winner)::DECIMAL / COUNT(*) FILTER (WHERE bid_price > 0)
            ELSE 0
        END,
        CASE
            WHEN COUNT(*) > 0
            THEN COUNT(*) FILTER (WHERE bid_price > 0)::DECIMAL / COUNT(*)
            ELSE 0
        END,
        CASE
            WHEN COUNT(*) > 0
            THEN COUNT(*) FILTER (WHERE is_error OR is_timeout)::DECIMAL / COUNT(*)
            ELSE 0
        END
    FROM bid_events
    WHERE timestamp >= hour_start AND timestamp < hour_end
    GROUP BY publisher_id, bidder_code
    ON CONFLICT (hour, publisher_id, bidder_code) DO UPDATE SET
        bid_request_count = EXCLUDED.bid_request_count,
        bid_count = EXCLUDED.bid_count,
        win_count = EXCLUDED.win_count,
        no_bid_count = EXCLUDED.no_bid_count,
        timeout_count = EXCLUDED.timeout_count,
        error_count = EXCLUDED.error_count,
        total_bid_value = EXCLUDED.total_bid_value,
        winning_bid_value = EXCLUDED.winning_bid_value,
        avg_bid_price = EXCLUDED.avg_bid_price,
        max_bid_price = EXCLUDED.max_bid_price,
        avg_latency_ms = EXCLUDED.avg_latency_ms,
        min_latency_ms = EXCLUDED.min_latency_ms,
        max_latency_ms = EXCLUDED.max_latency_ms,
        p50_latency_ms = EXCLUDED.p50_latency_ms,
        p95_latency_ms = EXCLUDED.p95_latency_ms,
        p99_latency_ms = EXCLUDED.p99_latency_ms,
        win_rate = EXCLUDED.win_rate,
        fill_rate = EXCLUDED.fill_rate,
        error_rate = EXCLUDED.error_rate,
        updated_at = NOW();

    GET DIAGNOSTICS rows_affected = ROW_COUNT;

    -- Also aggregate totals (all publishers, all bidders)
    INSERT INTO hourly_stats (
        hour, publisher_id, bidder_code,
        auction_count, bid_count, win_count, no_bid_count,
        timeout_count, error_count,
        total_bid_value, winning_bid_value
    )
    SELECT
        hour_start,
        '__all__',
        '__all__',
        COUNT(DISTINCT request_id),
        COUNT(*) FILTER (WHERE bid_price > 0 AND NOT is_no_bid),
        COUNT(*) FILTER (WHERE is_winner),
        COUNT(*) FILTER (WHERE is_no_bid),
        COUNT(*) FILTER (WHERE is_timeout),
        COUNT(*) FILTER (WHERE is_error),
        COALESCE(SUM(bid_price) FILTER (WHERE bid_price > 0), 0),
        COALESCE(SUM(bid_price) FILTER (WHERE is_winner), 0)
    FROM bid_events
    WHERE timestamp >= hour_start AND timestamp < hour_end
    ON CONFLICT (hour, publisher_id, bidder_code) DO UPDATE SET
        auction_count = EXCLUDED.auction_count,
        bid_count = EXCLUDED.bid_count,
        win_count = EXCLUDED.win_count,
        no_bid_count = EXCLUDED.no_bid_count,
        timeout_count = EXCLUDED.timeout_count,
        error_count = EXCLUDED.error_count,
        total_bid_value = EXCLUDED.total_bid_value,
        winning_bid_value = EXCLUDED.winning_bid_value,
        updated_at = NOW();

    RETURN rows_affected;
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- Function: Aggregate daily publisher stats
-- =====================================================

CREATE OR REPLACE FUNCTION aggregate_daily_publisher_stats(target_date DATE DEFAULT NULL)
RETURNS INTEGER AS $$
DECLARE
    day_start TIMESTAMPTZ;
    day_end TIMESTAMPTZ;
    rows_affected INTEGER := 0;
BEGIN
    -- Default to yesterday
    IF target_date IS NULL THEN
        target_date := CURRENT_DATE - INTERVAL '1 day';
    END IF;
    day_start := target_date::TIMESTAMPTZ;
    day_end := (target_date + INTERVAL '1 day')::TIMESTAMPTZ;

    INSERT INTO daily_publisher_stats (
        date, publisher_id,
        total_auctions, total_bids, total_wins,
        gross_revenue, avg_win_price, fill_rate,
        top_bidders
    )
    SELECT
        target_date,
        COALESCE(publisher_id, '__unknown__'),
        COUNT(DISTINCT request_id),
        COUNT(*) FILTER (WHERE bid_price > 0 AND NOT is_no_bid),
        COUNT(*) FILTER (WHERE is_winner),
        COALESCE(SUM(bid_price) FILTER (WHERE is_winner), 0),
        COALESCE(AVG(bid_price) FILTER (WHERE is_winner), 0),
        CASE
            WHEN COUNT(DISTINCT request_id) > 0
            THEN COUNT(*) FILTER (WHERE is_winner)::DECIMAL / COUNT(DISTINCT request_id)
            ELSE 0
        END,
        (
            SELECT JSONB_AGG(bidder_stats ORDER BY wins DESC)
            FROM (
                SELECT JSONB_BUILD_OBJECT(
                    'bidder', bidder_code,
                    'bids', COUNT(*) FILTER (WHERE bid_price > 0),
                    'wins', COUNT(*) FILTER (WHERE is_winner),
                    'revenue', COALESCE(SUM(bid_price) FILTER (WHERE is_winner), 0)
                ) as bidder_stats, COUNT(*) FILTER (WHERE is_winner) as wins
                FROM bid_events b2
                WHERE b2.timestamp >= day_start AND b2.timestamp < day_end
                  AND b2.publisher_id = bid_events.publisher_id
                GROUP BY b2.bidder_code
                ORDER BY wins DESC
                LIMIT 5
            ) top
        )
    FROM bid_events
    WHERE timestamp >= day_start AND timestamp < day_end
    GROUP BY publisher_id
    ON CONFLICT (date, publisher_id) DO UPDATE SET
        total_auctions = EXCLUDED.total_auctions,
        total_bids = EXCLUDED.total_bids,
        total_wins = EXCLUDED.total_wins,
        gross_revenue = EXCLUDED.gross_revenue,
        avg_win_price = EXCLUDED.avg_win_price,
        fill_rate = EXCLUDED.fill_rate,
        top_bidders = EXCLUDED.top_bidders,
        updated_at = NOW();

    GET DIAGNOSTICS rows_affected = ROW_COUNT;
    RETURN rows_affected;
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- Function: Create monthly partition (run monthly)
-- =====================================================

CREATE OR REPLACE FUNCTION create_monthly_partition(target_month DATE DEFAULT NULL)
RETURNS TEXT AS $$
DECLARE
    start_date DATE;
    end_date DATE;
    partition_name TEXT;
BEGIN
    -- Default to next month
    IF target_month IS NULL THEN
        start_date := DATE_TRUNC('month', CURRENT_DATE) + INTERVAL '1 month';
    ELSE
        start_date := DATE_TRUNC('month', target_month);
    END IF;
    end_date := start_date + INTERVAL '1 month';

    partition_name := 'auction_events_' || TO_CHAR(start_date, 'YYYY_MM');

    IF NOT EXISTS (
        SELECT 1 FROM pg_class WHERE relname = partition_name
    ) THEN
        EXECUTE FORMAT(
            'CREATE TABLE %I PARTITION OF auction_events FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
        RETURN 'Created partition: ' || partition_name;
    ELSE
        RETURN 'Partition already exists: ' || partition_name;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- Function: Drop old partitions (data retention)
-- =====================================================

CREATE OR REPLACE FUNCTION drop_old_partitions(retention_months INTEGER DEFAULT 3)
RETURNS TEXT AS $$
DECLARE
    cutoff_date DATE;
    partition_record RECORD;
    dropped_partitions TEXT := '';
BEGIN
    cutoff_date := DATE_TRUNC('month', CURRENT_DATE) - (retention_months || ' months')::INTERVAL;

    FOR partition_record IN
        SELECT inhrelid::regclass::text as partition_name
        FROM pg_inherits
        WHERE inhparent = 'auction_events'::regclass
          AND inhrelid::regclass::text LIKE 'auction_events_%'
          AND inhrelid::regclass::text != 'auction_events_default'
    LOOP
        -- Extract date from partition name and check if it's older than cutoff
        IF partition_record.partition_name ~ 'auction_events_\d{4}_\d{2}' THEN
            DECLARE
                partition_date DATE;
            BEGIN
                partition_date := TO_DATE(
                    SUBSTRING(partition_record.partition_name FROM 'auction_events_(\d{4}_\d{2})'),
                    'YYYY_MM'
                );

                IF partition_date < cutoff_date THEN
                    EXECUTE 'DROP TABLE ' || partition_record.partition_name;
                    dropped_partitions := dropped_partitions || partition_record.partition_name || ', ';
                END IF;
            EXCEPTION WHEN OTHERS THEN
                -- Skip partitions with unexpected naming
                NULL;
            END;
        END IF;
    END LOOP;

    IF dropped_partitions = '' THEN
        RETURN 'No partitions dropped';
    ELSE
        RETURN 'Dropped partitions: ' || RTRIM(dropped_partitions, ', ');
    END IF;
END;
$$ LANGUAGE plpgsql;

-- =====================================================
-- Useful views for querying
-- =====================================================

-- Real-time metrics (last hour)
CREATE OR REPLACE VIEW v_realtime_metrics AS
SELECT
    date_trunc('minute', timestamp) as minute,
    COUNT(DISTINCT request_id) as auctions,
    COUNT(*) FILTER (WHERE bid_price > 0 AND NOT is_no_bid) as bids,
    COUNT(*) FILTER (WHERE is_winner) as wins,
    COALESCE(SUM(bid_price) FILTER (WHERE is_winner), 0) as revenue,
    COALESCE(AVG(latency_ms), 0)::INTEGER as avg_latency_ms
FROM bid_events
WHERE timestamp > NOW() - INTERVAL '1 hour'
GROUP BY 1
ORDER BY 1 DESC;

-- Publisher leaderboard (today)
CREATE OR REPLACE VIEW v_publisher_leaderboard AS
SELECT
    COALESCE(publisher_id, '__unknown__') as publisher_id,
    COUNT(DISTINCT request_id) as auctions,
    COUNT(*) FILTER (WHERE is_winner) as wins,
    COALESCE(SUM(bid_price) FILTER (WHERE is_winner), 0) as revenue,
    CASE
        WHEN COUNT(DISTINCT request_id) > 0
        THEN ROUND(100.0 * COUNT(*) FILTER (WHERE is_winner) / COUNT(DISTINCT request_id), 2)
        ELSE 0
    END as fill_rate_pct
FROM bid_events
WHERE timestamp >= CURRENT_DATE
GROUP BY publisher_id
ORDER BY revenue DESC;

-- Bidder performance (today)
CREATE OR REPLACE VIEW v_bidder_performance AS
SELECT
    bidder_code,
    COUNT(*) as total_requests,
    COUNT(*) FILTER (WHERE bid_price > 0 AND NOT is_no_bid) as bids,
    COUNT(*) FILTER (WHERE is_winner) as wins,
    COUNT(*) FILTER (WHERE is_timeout) as timeouts,
    COUNT(*) FILTER (WHERE is_error) as errors,
    COALESCE(AVG(bid_price) FILTER (WHERE bid_price > 0), 0) as avg_bid,
    COALESCE(AVG(latency_ms), 0)::INTEGER as avg_latency_ms,
    COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY latency_ms)::INTEGER, 0) as p95_latency_ms
FROM bid_events
WHERE timestamp >= CURRENT_DATE
GROUP BY bidder_code
ORDER BY wins DESC;

-- =====================================================
-- Comments
-- =====================================================

COMMENT ON TABLE auction_events IS 'Raw auction events partitioned by month for efficient querying and retention';
COMMENT ON TABLE bid_events IS 'Denormalized bid events optimized for revenue and performance queries';
COMMENT ON TABLE hourly_stats IS 'Pre-aggregated hourly metrics for fast dashboard queries';
COMMENT ON TABLE daily_publisher_stats IS 'Daily rollup of publisher performance for business reporting';
COMMENT ON FUNCTION aggregate_hourly_stats IS 'Aggregates bid_events into hourly_stats - run hourly via cron';
COMMENT ON FUNCTION aggregate_daily_publisher_stats IS 'Aggregates daily publisher metrics - run daily via cron';
COMMENT ON FUNCTION create_monthly_partition IS 'Creates new monthly partition for auction_events - run monthly';
COMMENT ON FUNCTION drop_old_partitions IS 'Drops partitions older than retention period - run monthly';
