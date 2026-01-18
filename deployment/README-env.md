# Environment Variables - Complete Reference

## Overview

This document explains **every environment variable** used by TNE Catalyst. Understanding these settings is critical for proper deployment and operation.

## Environment Files

### Available Templates

```
deployment/
├── .env.dev          ← Local development (localhost)
├── .env.production   ← Production (catalyst.springwire.ai)
└── .env.staging      ← Staging (5% traffic testing)
```

### Usage

```bash
# Development
docker compose --env-file .env.dev up -d

# Production (100% traffic)
docker compose --env-file .env.production up -d

# Traffic splitting (95% prod, 5% staging)
docker compose -f docker-compose-split.yml up -d
```

---

## Server Configuration

### PBS_HOST_URL

**Purpose**: The public URL where Catalyst is accessible.

**Format**: Full URL with protocol (http:// or https://)

**Examples**:
```bash
PBS_HOST_URL=http://localhost:8000          # Development
PBS_HOST_URL=https://catalyst.springwire.ai # Production
```

**Used For**:
- Callback URLs in OpenRTB responses
- VAST tag generation
- External references to the service

**Impact**: If wrong, callbacks and tracking pixels won't work.

### PBS_PORT

**Purpose**: Port the server listens on inside the container.

**Default**: 8000

**Examples**:
```bash
PBS_PORT=8000  # Standard
PBS_PORT=9000  # Custom port
```

**When to Change**: Rarely. Only if port 8000 conflicts with other services.

**Note**: Docker handles external → internal port mapping. This is the internal port.

---

## Database Configuration

### DB_HOST

**Purpose**: PostgreSQL database hostname or IP.

**Examples**:
```bash
DB_HOST=localhost           # Local development
DB_HOST=postgres            # Docker container name
DB_HOST=db.example.com      # Remote database
DB_HOST=192.168.1.10        # IP address
```

**Docker Note**: Use container name (e.g., `postgres`) not `localhost` when running in containers.

### DB_PORT

**Purpose**: PostgreSQL port.

**Default**: 5432 (PostgreSQL standard)

**When to Change**: Only if your database runs on a non-standard port.

### DB_NAME

**Purpose**: Database name to connect to.

**Examples**:
```bash
DB_NAME=catalyst_dev        # Development
DB_NAME=catalyst_production # Production
DB_NAME=catalyst_test       # Testing
```

**Important**: Create this database before starting Catalyst:
```sql
CREATE DATABASE catalyst_production;
```

### DB_USER

**Purpose**: PostgreSQL username.

**Security**: Use strong, unique credentials for production.

**Examples**:
```bash
DB_USER=catalyst_dev   # Development
DB_USER=catalyst_prod  # Production
```

**Best Practice**: Create dedicated database user with limited permissions:
```sql
CREATE USER catalyst_prod WITH PASSWORD 'strong_password';
GRANT ALL PRIVILEGES ON DATABASE catalyst_production TO catalyst_prod;
```

### DB_PASSWORD

**Purpose**: PostgreSQL password.

**Security**:
- ⚠️ CRITICAL: Change default passwords before deployment
- Use strong passwords (20+ characters, mixed case, numbers, symbols)
- Never commit passwords to git

**Examples**:
```bash
DB_PASSWORD=dev_password                    # Development (weak, okay)
DB_PASSWORD=X9mK#pL2$vN8qR4@wT6yZ1bF3hJ7  # Production (strong)
```

**Best Practice**: Use password manager or secrets management service.

### DB_SSL_MODE

**Purpose**: PostgreSQL SSL/TLS connection security.

**Options**:
- `disable` - No SSL (development only)
- `require` - SSL required, but don't verify certificate
- `verify-ca` - Verify certificate authority
- `verify-full` - Full certificate verification (most secure)

**Recommendations**:
```bash
DB_SSL_MODE=disable      # Development (localhost)
DB_SSL_MODE=require      # Production (minimum)
DB_SSL_MODE=verify-full  # Production (recommended)
```

**Trade-offs**:
- `disable`: Fast, but insecure
- `require`: Encrypted connection, simple setup
- `verify-full`: Most secure, requires CA certificates

### DB_MAX_OPEN_CONNS

**Purpose**: Maximum number of open database connections.

**Default**: 25 (development), 100 (production)

**Impact**:
- Too low: Bottleneck under load
- Too high: Database overload

**Recommendations**:
```bash
DB_MAX_OPEN_CONNS=25   # Development
DB_MAX_OPEN_CONNS=100  # Production
DB_MAX_OPEN_CONNS=50   # Staging (5% traffic)
```

**Formula**: `(expected_concurrent_requests / 2) + buffer`

### DB_MAX_IDLE_CONNS

**Purpose**: Maximum idle connections kept in pool.

**Default**: 5 (development), 25 (production)

**Impact**:
- Too low: Constant connection churn
- Too high: Wasted resources

**Recommendations**:
```bash
DB_MAX_IDLE_CONNS=5    # Development
DB_MAX_IDLE_CONNS=25   # Production
```

**Rule of Thumb**: ~25% of max_open_conns

### DB_CONN_MAX_LIFETIME

**Purpose**: Maximum time a connection can be reused.

**Format**: Duration (e.g., 300s, 5m, 1h)

**Default**: 300s (5 minutes) development, 600s (10 minutes) production

**Why**: Prevents stale connections, forces reconnection periodically.

**Examples**:
```bash
DB_CONN_MAX_LIFETIME=300s  # 5 minutes
DB_CONN_MAX_LIFETIME=10m   # 10 minutes
DB_CONN_MAX_LIFETIME=1h    # 1 hour
```

---

## Redis Configuration

### REDIS_HOST

**Purpose**: Redis server hostname or IP.

**Examples**:
```bash
REDIS_HOST=localhost       # Local development
REDIS_HOST=redis-prod      # Docker container (production)
REDIS_HOST=redis-staging   # Docker container (staging)
REDIS_HOST=redis.example.com  # Remote Redis
```

**Important**: Production and staging use **separate Redis instances** for data isolation.

### REDIS_PORT

**Purpose**: Redis port.

**Default**: 6379 (Redis standard)

### REDIS_PASSWORD

**Purpose**: Redis authentication password.

**Security**:
- Leave empty for development (no password)
- Set strong password for production
- Change default passwords

**Examples**:
```bash
REDIS_PASSWORD=                          # Development (no password)
REDIS_PASSWORD=Y8nL#kP4$vM7qR2@wT5xZ9bF # Production
```

**Setting Password**: Configured in docker-compose.yml redis command:
```yaml
command: redis-server --requirepass your_password_here
```

### REDIS_DB

**Purpose**: Redis database number (0-15 by default).

**Default**: 0

**When to Change**: Use different DB numbers if sharing Redis instance (not recommended).

**Examples**:
```bash
REDIS_DB=0  # Production
REDIS_DB=1  # Staging (if sharing Redis - not recommended)
```

**Best Practice**: Use separate Redis instances instead of separate DB numbers.

### REDIS_POOL_SIZE

**Purpose**: Maximum Redis connections in pool.

**Default**: 10 (dev), 50 (prod), 20 (staging)

**Impact**:
- Too low: Redis operations queued/delayed
- Too high: Excessive connections

**Recommendations**:
```bash
REDIS_POOL_SIZE=10  # Development
REDIS_POOL_SIZE=50  # Production
REDIS_POOL_SIZE=20  # Staging (5% traffic)
```

### REDIS_IDLE_TIMEOUT

**Purpose**: Close idle connections after this duration.

**Default**: 300s (5 minutes)

**Format**: Duration (e.g., 300s, 5m)

**Why**: Cleans up unused connections, saves resources.

### REDIS_POOL_TIMEOUT

**Purpose**: Max time to wait for connection from pool.

**Default**: 4s

**Impact**: If pool exhausted, wait this long before failing.

**Recommendations**:
```bash
REDIS_POOL_TIMEOUT=4s   # Standard
REDIS_POOL_TIMEOUT=10s  # If expecting high contention
```

### REDIS_AUCTION_TTL

**Purpose**: Time-to-live for auction state data in Redis.

**Default**: 300 seconds (5 minutes)

**Why**: Auction data is temporary, clean up after completion.

**When to Change**:
- Increase if auctions take longer
- Decrease to save memory

### REDIS_CACHE_TTL

**Purpose**: Time-to-live for cached data (config, user data, etc.).

**Default**: 3600 seconds (1 hour)

**Impact**:
- Lower: More database queries, fresher data
- Higher: Less database load, staler data

---

## IDR (Intelligent Demand Router)

### IDR_ENABLED

**Purpose**: Enable/disable ML-powered demand routing.

**Default**: false

**Values**: true, false

**Important**:
- ✅ Works fine with `false` - IDR is optional
- IDR requires separate ML service running
- Enable only after IDR service is deployed

**Examples**:
```bash
IDR_ENABLED=false  # Initial deployment (recommended)
IDR_ENABLED=true   # After IDR service ready
```

### IDR_URL

**Purpose**: URL of IDR service.

**Only Used**: When IDR_ENABLED=true

**Examples**:
```bash
IDR_URL=http://localhost:8080              # Development
IDR_URL=https://idr.catalyst.springwire.ai # Production
```

### IDR_TIMEOUT

**Purpose**: Max time to wait for IDR response.

**Default**: 500ms

**Format**: Duration (e.g., 500ms, 1s)

**Trade-offs**:
- Lower: Faster auctions, but IDR may timeout
- Higher: More accurate routing, slower auctions

**Recommendations**:
```bash
IDR_TIMEOUT=500ms  # Standard (recommended)
IDR_TIMEOUT=1s     # If IDR is slow but valuable
```

### IDR_RETRY_ATTEMPTS

**Purpose**: How many times to retry failed IDR calls.

**Default**: 2 (dev), 3 (prod)

**Trade-offs**:
- More retries: Higher success rate, slower
- Fewer retries: Faster, but may miss IDR benefits

### IDR_FALLBACK_ENABLED

**Purpose**: Continue auction if IDR fails.

**Default**: true

**Values**: true, false

**Important**:
- `true` - Auction proceeds without IDR (recommended)
- `false` - Auction fails if IDR fails (risky)

**Recommendation**: Always `true` for production reliability.

---

## IVT (Invalid Traffic) Detection

### IVT_BLOCKING_ENABLED

**Purpose**: Block requests identified as invalid traffic.

**Default**: false (production - monitoring only), true (staging - test blocking)

**Values**: true, false

**Strategy**:
1. Start with `false` (monitor only)
2. Review detection accuracy
3. Test with `true` in staging
4. Enable in production once confident

**Examples**:
```bash
IVT_BLOCKING_ENABLED=false  # Production (monitor)
IVT_BLOCKING_ENABLED=true   # Staging (test blocking)
```

**Impact**:
- `true`: Blocks suspicious traffic (may have false positives)
- `false`: Logs only (safe, but allows invalid traffic)

### IVT_ALLOWED_COUNTRIES

**Purpose**: Restrict traffic by country (geo-blocking).

**Format**: Comma-separated ISO 3166-1 alpha-2 country codes

**Special**: `*` allows all countries

**Examples**:
```bash
IVT_ALLOWED_COUNTRIES=*           # Allow all countries
IVT_ALLOWED_COUNTRIES=US,GB,CA    # USA, UK, Canada only
IVT_ALLOWED_COUNTRIES=US          # USA only
```

**Country Codes**: US, GB, CA, AU, DE, FR, JP, etc.

**Use Cases**:
- Compliance (GDPR - EU only)
- Targeting (specific markets)
- IVT reduction (high-fraud countries)

**Warning**: May block legitimate users. Test in staging first.

### IVT_CHECK_USER_AGENT

**Purpose**: Enable user agent validation.

**Default**: true

**What It Checks**:
- Empty user agents
- Suspicious bot patterns
- Known scraper signatures

**Examples**:
```bash
IVT_CHECK_USER_AGENT=true   # Recommended
IVT_CHECK_USER_AGENT=false  # Development only
```

### IVT_CHECK_IP_REPUTATION

**Purpose**: Check IP against known bad IP lists.

**Default**: true

**What It Checks**:
- Known bot IPs
- VPN/proxy IPs
- Data center IPs
- Historical fraud IPs

**Performance Impact**: Requires external lookup (cached).

### IVT_CHECK_GEO_MISMATCH

**Purpose**: Detect geo-location inconsistencies.

**Default**: true

**What It Checks**:
- IP location vs declared location
- Timezone inconsistencies

**Use Case**: Detect spoofed locations.

### IVT_CHECK_SUSPICIOUS_PATTERNS

**Purpose**: ML-based pattern detection.

**Default**: true

**What It Checks**:
- Unusual request patterns
- Repeated failed auctions
- Abnormal bid behavior

### IVT_SUSPICIOUS_THRESHOLD

**Purpose**: Confidence threshold for IVT detection (0.0 - 1.0).

**Format**: Float (0.0 to 1.0)

**Meaning**:
- 0.0 = Block everything (too strict)
- 0.5 = Moderate detection
- 1.0 = Block nothing (too permissive)

**Recommendations**:
```bash
IVT_SUSPICIOUS_THRESHOLD=0.8  # Production (conservative)
IVT_SUSPICIOUS_THRESHOLD=0.6  # Staging (more aggressive)
IVT_SUSPICIOUS_THRESHOLD=0.7  # Development (balanced)
```

**Strategy**: Start high (0.8), lower gradually based on false positive rate.

---

## CORS (Cross-Origin Resource Sharing)

### CORS_ALLOWED_ORIGINS

**Purpose**: Which domains can make requests to Catalyst.

**Critical**: Must be configured correctly for Prebid.js to work.

**Format**: Comma-separated URLs with protocol

**Examples**:
```bash
# Allow all (development only)
CORS_ALLOWED_ORIGINS=*

# Single domain
CORS_ALLOWED_ORIGINS=https://yourpublisher.com

# Multiple domains
CORS_ALLOWED_ORIGINS=https://site1.com,https://site2.com

# Wildcard subdomain (use carefully)
CORS_ALLOWED_ORIGINS=https://*.yourpublisher.com

# Mixed
CORS_ALLOWED_ORIGINS=https://yourpublisher.com,https://*.partner.com
```

**Security**:
- ⚠️ Never use `*` in production
- Use specific domains only
- Include all your publisher sites

**Common Issue**: If Prebid.js can't reach Catalyst, check CORS.

### CORS_ALLOW_CREDENTIALS

**Purpose**: Allow cookies/credentials in CORS requests.

**Default**: true

**Values**: true, false

**When to Change**: Rarely. Keep `true` unless you have specific reason.

### CORS_MAX_AGE

**Purpose**: How long browsers cache CORS preflight responses.

**Default**: 3600 (1 hour)

**Format**: Seconds

**Impact**:
- Higher: Fewer preflight requests, less traffic
- Lower: More up-to-date CORS policy

---

## Auction Configuration

### AUCTION_TIMEOUT

**Purpose**: Maximum time to wait for all bidders to respond.

**Default**: 2s (2 seconds)

**Format**: Duration (e.g., 2s, 1500ms)

**Trade-offs**:
- Lower (1s): Faster page load, may miss bids
- Higher (3s): More bids, slower page load

**Recommendations**:
```bash
AUCTION_TIMEOUT=1s   # Fast, may lose some bids
AUCTION_TIMEOUT=2s   # Balanced (recommended)
AUCTION_TIMEOUT=3s   # Maximum bids, slower
```

**Impact**: Directly affects user experience (page load time).

### AUCTION_MAX_BIDDERS

**Purpose**: Maximum number of bidders to call per auction.

**Default**: 10 (dev), 15 (prod)

**Trade-offs**:
- More bidders: Higher competition, more revenue, slower
- Fewer bidders: Faster, but less competition

**Recommendations**:
```bash
AUCTION_MAX_BIDDERS=5   # Fast auctions
AUCTION_MAX_BIDDERS=10  # Balanced
AUCTION_MAX_BIDDERS=15  # Maximum competition
```

**Note**: With parallel bidding, all bidders called simultaneously.

### AUCTION_PARALLEL_BIDDING

**Purpose**: Call all bidders simultaneously vs sequentially.

**Default**: true

**Values**: true, false

**Impact**:
- `true`: All bidders called at once (recommended)
- `false`: One by one (slow, rarely used)

**Recommendation**: Always `true` for performance.

---

## Rate Limiting

### RATE_LIMIT_GENERAL

**Purpose**: General endpoint rate limit (requests per second).

**Default**: 1000 (dev), 100 (prod)

**Format**: Requests per second

**Examples**:
```bash
RATE_LIMIT_GENERAL=1000  # Development (permissive)
RATE_LIMIT_GENERAL=100   # Production (standard)
RATE_LIMIT_GENERAL=500   # High-traffic production
```

**When to Adjust**:
- Increase: If legitimate traffic is being blocked
- Decrease: If under attack

### RATE_LIMIT_AUCTION

**Purpose**: Auction endpoint rate limit (requests per second).

**Default**: 500 (dev), 50 (prod)

**Why Lower**: Auctions are expensive, limit abuse.

**Examples**:
```bash
RATE_LIMIT_AUCTION=500  # Development
RATE_LIMIT_AUCTION=50   # Production
RATE_LIMIT_AUCTION=100  # High-traffic production
```

### RATE_LIMIT_BURST

**Purpose**: Allow temporary traffic spikes above rate limit.

**Default**: 100 (dev), 50 (prod)

**How It Works**: Allow brief bursts, then enforce rate limit.

**Examples**:
```bash
RATE_LIMIT_BURST=100  # Development (permissive)
RATE_LIMIT_BURST=50   # Production
```

**Use Case**: Handle legitimate traffic spikes (e.g., homepage refresh).

---

## Logging

### LOG_LEVEL

**Purpose**: How verbose should logging be.

**Options**:
- `debug` - Everything (very verbose)
- `info` - Normal operations
- `warn` - Warnings and errors only
- `error` - Errors only

**Recommendations**:
```bash
LOG_LEVEL=debug  # Development (see everything)
LOG_LEVEL=info   # Production (standard)
LOG_LEVEL=warn   # Production (less verbose)
```

**Performance**: `debug` has overhead, use `info` in production.

### LOG_FORMAT

**Purpose**: Log output format.

**Options**:
- `text` - Human-readable (development)
- `json` - Machine-readable (production, log aggregation)

**Recommendations**:
```bash
LOG_FORMAT=text  # Development (easier to read)
LOG_FORMAT=json  # Production (for log aggregation)
```

**Why JSON**: Easier to parse, index, and search in log management tools.

### LOG_REQUESTS

**Purpose**: Log every HTTP request.

**Default**: true

**Values**: true, false

**Impact**:
- `true`: See all traffic (useful for debugging)
- `false`: Less log volume

**Production**: Usually `true` for audit trail.

### LOG_METRICS

**Purpose**: Log performance metrics (latency, etc.).

**Default**: true

**Values**: true, false

**Use Case**: Performance monitoring and optimization.

---

## Security & Headers

### SECURITY_HSTS_ENABLED

**Purpose**: Enable HTTP Strict Transport Security header.

**Default**: false (dev), true (prod)

**Values**: true, false

**What It Does**: Forces browsers to use HTTPS only.

**Recommendation**: `true` for production (if using HTTPS).

### SECURITY_CSRF_ENABLED

**Purpose**: Enable CSRF (Cross-Site Request Forgery) protection.

**Default**: false (dev), true (prod)

**Values**: true, false

**Recommendation**: `true` for production security.

### TRUST_PROXY

**Purpose**: Trust X-Forwarded-* headers from reverse proxy.

**Default**: true

**Values**: true, false

**Important**: Must be `true` when behind nginx reverse proxy.

**Why**: Catalyst needs real client IP from proxy headers.

---

## Feature Flags

### FEATURE_EXPERIMENTAL_BIDDERS

**Purpose**: Enable experimental/beta bidder adapters.

**Default**: true (dev/staging), false (prod)

**Values**: true, false

**Use Case**: Test new bidders before production.

### FEATURE_ADVANCED_TARGETING

**Purpose**: Enable advanced targeting features.

**Default**: true

**Values**: true, false

### FEATURE_DEBUG_ENDPOINTS

**Purpose**: Enable debug/diagnostic endpoints.

**Default**: true (dev/staging), false (prod)

**Values**: true, false

**Security**: Disable in production (may expose internal state).

---

## Error Tracking (Sentry)

### SENTRY_DSN

**Purpose**: Sentry Data Source Name for error tracking.

**Format**: Full Sentry DSN URL

**Examples**:
```bash
SENTRY_DSN=                                                    # Disabled (default)
SENTRY_DSN=https://xxx@o123.ingest.sentry.io/456              # Production
```

**How to Get**:
1. Create account at https://sentry.io (free tier available)
2. Create a new Go project
3. Copy the DSN from project settings

**What It Does**:
- Captures exceptions with stack traces
- Adds request context to errors
- Groups similar errors automatically
- Alerts on new/recurring issues

### SENTRY_ENVIRONMENT

**Purpose**: Environment name shown in Sentry dashboard.

**Default**: development

**Examples**:
```bash
SENTRY_ENVIRONMENT=development  # Local development
SENTRY_ENVIRONMENT=staging      # Staging server
SENTRY_ENVIRONMENT=production   # Production server
```

**Use Case**: Filter errors by environment in Sentry dashboard.

### SENTRY_RELEASE

**Purpose**: Release/version identifier shown in Sentry.

**Default**: 1.0.0

**Examples**:
```bash
SENTRY_RELEASE=1.0.0
SENTRY_RELEASE=v2.3.1
SENTRY_RELEASE=2024-01-15-abc123  # Date + commit hash
```

**Use Case**: Track which release introduced errors, identify regressions.

### SENTRY_DEBUG

**Purpose**: Enable Sentry debug mode.

**Default**: false

**Values**: true, false

**When to Enable**: Only for debugging Sentry integration issues.

---

## Alerting Webhooks

### ALERT_SLACK_WEBHOOK_URL

**Purpose**: Slack incoming webhook URL for alerts.

**Format**: Full Slack webhook URL

**Examples**:
```bash
ALERT_SLACK_WEBHOOK_URL=                                                      # Disabled
ALERT_SLACK_WEBHOOK_URL=https://hooks.slack.com/services/T00/B00/xxxx        # Enabled
```

**How to Get**:
1. Go to Slack App Directory → Incoming Webhooks
2. Create new webhook for your channel
3. Copy the webhook URL

**What Gets Sent**: Error rate alerts, latency alerts, circuit breaker state changes.

### ALERT_DISCORD_WEBHOOK_URL

**Purpose**: Discord webhook URL for alerts.

**Format**: Full Discord webhook URL

**Examples**:
```bash
ALERT_DISCORD_WEBHOOK_URL=                                                    # Disabled
ALERT_DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/123456/abcdef     # Enabled
```

**How to Get**:
1. Server Settings → Integrations → Webhooks
2. Create New Webhook
3. Copy the webhook URL

### ALERT_PAGERDUTY_ROUTING_KEY

**Purpose**: PagerDuty Events API v2 routing key for critical alerts.

**Format**: PagerDuty integration key (32 characters)

**Examples**:
```bash
ALERT_PAGERDUTY_ROUTING_KEY=                                  # Disabled
ALERT_PAGERDUTY_ROUTING_KEY=R01234567890ABCDEFGHIJKLMNOP     # Enabled
```

**How to Get**:
1. PagerDuty → Services → Your Service → Integrations
2. Add Integration → Events API v2
3. Copy the Integration Key

**Important**: Only critical alerts (circuit breaker open) are sent to PagerDuty.

### ALERT_WEBHOOK_URL

**Purpose**: Generic webhook URL for custom alerting systems.

**Format**: Any HTTP/HTTPS URL that accepts POST JSON

**Examples**:
```bash
ALERT_WEBHOOK_URL=                                            # Disabled
ALERT_WEBHOOK_URL=https://your-system.com/alerts/webhook     # Enabled
```

**Payload Format**: JSON with alert details (name, severity, message, timestamp, tags).

### ALERT_SERVICE_NAME

**Purpose**: Service name included in alert payloads.

**Default**: pbs

**Examples**:
```bash
ALERT_SERVICE_NAME=pbs
ALERT_SERVICE_NAME=catalyst-prod
ALERT_SERVICE_NAME=prebid-server
```

### ALERT_ENVIRONMENT

**Purpose**: Environment name included in alert payloads.

**Default**: development

**Examples**:
```bash
ALERT_ENVIRONMENT=development
ALERT_ENVIRONMENT=staging
ALERT_ENVIRONMENT=production
```

---

## Alert Thresholds

### ALERT_ERROR_RATE_THRESHOLD

**Purpose**: Error rate percentage threshold for alerts.

**Default**: 5.0 (5%)

**Format**: Float (percentage, 0-100)

**Examples**:
```bash
ALERT_ERROR_RATE_THRESHOLD=5.0   # Alert when >5% of requests fail
ALERT_ERROR_RATE_THRESHOLD=2.0   # More sensitive (2%)
ALERT_ERROR_RATE_THRESHOLD=10.0  # Less sensitive (10%)
```

### ALERT_LATENCY_THRESHOLD_MS

**Purpose**: Average latency threshold for alerts (milliseconds).

**Default**: 1000 (1 second)

**Format**: Float (milliseconds)

**Examples**:
```bash
ALERT_LATENCY_THRESHOLD_MS=1000  # Alert when avg latency >1s
ALERT_LATENCY_THRESHOLD_MS=500   # More sensitive (500ms)
ALERT_LATENCY_THRESHOLD_MS=2000  # Less sensitive (2s)
```

### ALERT_RATE_LIMIT_THRESHOLD

**Purpose**: Rate limit rejections per minute threshold.

**Default**: 100

**Format**: Integer (rejections per minute)

**Examples**:
```bash
ALERT_RATE_LIMIT_THRESHOLD=100   # Alert when >100 rejections/min
ALERT_RATE_LIMIT_THRESHOLD=50    # More sensitive
ALERT_RATE_LIMIT_THRESHOLD=200   # Less sensitive
```

### ALERT_CIRCUIT_BREAKER

**Purpose**: Alert when circuit breaker opens.

**Default**: true

**Values**: true, false

**Recommendation**: Always `true` - circuit breaker opening indicates IDR service issues.

---

## Monitoring & Profiling

### PPROF_ENABLED

**Purpose**: Enable Go profiling endpoints (CPU, memory analysis).

**Default**: true (dev), false (prod)

**Values**: true, false

**Endpoints**: /debug/pprof/*

**Security**: ⚠️ Disable in production (exposes internal state).

**Use Case**: Performance debugging and optimization.

### DEBUG_ENDPOINTS

**Purpose**: Enable debugging endpoints.

**Default**: true (dev/staging), false (prod)

**Values**: true, false

**Security**: Disable in production.

---

## Performance Tuning

### GOMAXPROCS

**Purpose**: Number of OS threads for Go runtime.

**Default**: 0 (use all available CPUs)

**Format**: Integer (0 = auto)

**When to Change**: Rarely. Only for specific performance tuning.

### GOGC

**Purpose**: Garbage collection target percentage.

**Default**: 100 (Go default)

**Format**: Integer percentage

**Impact**:
- Lower (50): More frequent GC, lower memory
- Higher (200): Less frequent GC, higher memory

**When to Tune**: If experiencing GC pauses or memory issues.

---

## Environment-Specific Settings

### ENV_NAME

**Purpose**: Name of current environment.

**Examples**:
```bash
ENV_NAME=development
ENV_NAME=staging
ENV_NAME=production
```

**Use Case**: Logging, metrics tagging, debugging.

### STAGING_MODE

**Purpose**: Mark this instance as staging.

**Default**: false (prod), true (staging)

**Values**: true, false

**Use Case**: Different behavior in staging (more logging, etc.).

### ADD_DEBUG_HEADERS

**Purpose**: Add debug information to HTTP response headers.

**Default**: true (dev/staging), false (prod)

**Values**: true, false

**What It Adds**:
- X-Backend (which container handled request)
- X-Request-ID (request tracking)
- Timing information

---

## Quick Reference

### Development (.env.dev)
```bash
PBS_HOST_URL=http://localhost:8000
DB_HOST=localhost
DB_SSL_MODE=disable
REDIS_HOST=localhost
IDR_ENABLED=false
IVT_BLOCKING_ENABLED=false
CORS_ALLOWED_ORIGINS=*
LOG_LEVEL=debug
LOG_FORMAT=text
```

### Production (.env.production)
```bash
PBS_HOST_URL=https://catalyst.springwire.ai
DB_HOST=postgres
DB_SSL_MODE=require
REDIS_HOST=redis-prod
IDR_ENABLED=false
IVT_BLOCKING_ENABLED=false
CORS_ALLOWED_ORIGINS=https://yourpublisher.com
LOG_LEVEL=info
LOG_FORMAT=json
```

### Staging (.env.staging)
```bash
PBS_HOST_URL=https://catalyst.springwire.ai
DB_HOST=postgres
DB_SSL_MODE=require
REDIS_HOST=redis-staging  # Separate Redis!
IDR_ENABLED=false
IVT_BLOCKING_ENABLED=true  # Test blocking here
CORS_ALLOWED_ORIGINS=https://yourpublisher.com
LOG_LEVEL=debug
LOG_FORMAT=json
```

---

## Common Configuration Scenarios

### Scenario 1: High-Traffic Production
```bash
DB_MAX_OPEN_CONNS=200
REDIS_POOL_SIZE=100
RATE_LIMIT_GENERAL=500
RATE_LIMIT_AUCTION=200
AUCTION_MAX_BIDDERS=20
```

### Scenario 2: Low-Resource Server
```bash
DB_MAX_OPEN_CONNS=25
REDIS_POOL_SIZE=10
RATE_LIMIT_GENERAL=50
RATE_LIMIT_AUCTION=25
AUCTION_MAX_BIDDERS=8
```

### Scenario 3: Testing IVT Aggressively
```bash
IVT_BLOCKING_ENABLED=true
IVT_ALLOWED_COUNTRIES=US,GB,CA
IVT_SUSPICIOUS_THRESHOLD=0.5
LOG_LEVEL=debug
```

### Scenario 4: Maximum Performance
```bash
AUCTION_TIMEOUT=1s
AUCTION_MAX_BIDDERS=8
AUCTION_PARALLEL_BIDDING=true
LOG_LEVEL=warn
LOG_REQUESTS=false
```

---

## Security Checklist

Before production deployment:

- [ ] Changed `DB_PASSWORD` to strong password
- [ ] Changed `REDIS_PASSWORD` to strong password
- [ ] Set `CORS_ALLOWED_ORIGINS` to specific domains (not `*`)
- [ ] Set `DB_SSL_MODE=require` or higher
- [ ] Set `LOG_LEVEL=info` (not `debug`)
- [ ] Set `PPROF_ENABLED=false`
- [ ] Set `DEBUG_ENDPOINTS=false`
- [ ] Set `FEATURE_DEBUG_ENDPOINTS=false`
- [ ] Verified SSL certificates in `./ssl/` directory

---

## Troubleshooting

### Container won't start
**Check**: Database and Redis credentials are correct
```bash
docker compose logs catalyst-prod
```

### CORS errors in browser
**Check**: `CORS_ALLOWED_ORIGINS` includes your publisher domain
**Fix**: Add your domain to the list

### Slow auctions
**Check**:
- `AUCTION_TIMEOUT` (try reducing)
- `AUCTION_MAX_BIDDERS` (try reducing)
- `IDR_ENABLED` (disable if slow)

### High memory usage
**Check**:
- `REDIS_CACHE_TTL` (reduce to free memory)
- `DB_MAX_OPEN_CONNS` (reduce pool size)
- `REDIS_POOL_SIZE` (reduce pool size)

### Rate limiting blocking users
**Check**:
- `RATE_LIMIT_GENERAL` (increase)
- `RATE_LIMIT_AUCTION` (increase)
- `RATE_LIMIT_BURST` (increase)

### Can't connect to database
**Check**:
- `DB_HOST` (use container name, not localhost)
- `DB_SSL_MODE` (try `disable` first, then `require`)
- Database is running: `docker compose ps`

---

**Last Updated**: 2025-01-13
**Files**: .env.dev, .env.production, .env.staging
**Deployment**: catalyst.springwire.ai
