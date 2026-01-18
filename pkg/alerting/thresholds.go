// Package alerting provides threshold-based alerting for metrics monitoring
package alerting

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// ThresholdConfig defines alerting thresholds
type ThresholdConfig struct {
	// Error rate threshold (percentage, 0-100)
	ErrorRateThreshold float64 `json:"error_rate_threshold"`
	// High latency threshold (milliseconds)
	LatencyThresholdMs float64 `json:"latency_threshold_ms"`
	// Circuit breaker open alert
	CircuitBreakerAlert bool `json:"circuit_breaker_alert"`
	// Rate limit rejection threshold (per minute)
	RateLimitThreshold int `json:"rate_limit_threshold"`
	// Check interval
	CheckInterval time.Duration `json:"check_interval"`
}

// DefaultThresholdConfig returns sensible defaults for thresholds
func DefaultThresholdConfig() ThresholdConfig {
	return ThresholdConfig{
		ErrorRateThreshold:  parseFloatEnv("ALERT_ERROR_RATE_THRESHOLD", 5.0),    // 5% error rate
		LatencyThresholdMs:  parseFloatEnv("ALERT_LATENCY_THRESHOLD_MS", 1000.0), // 1 second
		CircuitBreakerAlert: os.Getenv("ALERT_CIRCUIT_BREAKER") != "false",
		RateLimitThreshold:  parseIntEnv("ALERT_RATE_LIMIT_THRESHOLD", 100),
		CheckInterval:       1 * time.Minute,
	}
}

// MetricsSource provides metrics data for threshold checking
type MetricsSource interface {
	// GetErrorRate returns the error rate as a percentage (0-100)
	GetErrorRate() float64
	// GetAverageLatencyMs returns average latency in milliseconds
	GetAverageLatencyMs() float64
	// IsCircuitBreakerOpen returns true if the circuit breaker is open
	IsCircuitBreakerOpen() bool
	// GetRateLimitRejections returns the count of rate-limited requests
	GetRateLimitRejections() int64
	// GetTotalRequests returns total request count
	GetTotalRequests() int64
}

// ThresholdMonitor monitors metrics and triggers alerts
type ThresholdMonitor struct {
	config        ThresholdConfig
	alertManager  *Manager
	metricsSource MetricsSource
	mu            sync.Mutex
	stopCh        chan struct{}
	running       bool

	// Track previous values for rate calculations
	lastCheck            time.Time
	lastRateLimitRejects int64
	lastTotalRequests    int64
}

// NewThresholdMonitor creates a new threshold monitor
func NewThresholdMonitor(cfg ThresholdConfig, alertMgr *Manager, metrics MetricsSource) *ThresholdMonitor {
	return &ThresholdMonitor{
		config:        cfg,
		alertManager:  alertMgr,
		metricsSource: metrics,
		stopCh:        make(chan struct{}),
		lastCheck:     time.Now(),
	}
}

// Start begins monitoring thresholds in a background goroutine
func (tm *ThresholdMonitor) Start() {
	tm.mu.Lock()
	if tm.running {
		tm.mu.Unlock()
		return
	}
	tm.running = true
	tm.mu.Unlock()

	go tm.monitorLoop()
}

// Stop stops the threshold monitor
func (tm *ThresholdMonitor) Stop() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if !tm.running {
		return
	}

	close(tm.stopCh)
	tm.running = false
}

func (tm *ThresholdMonitor) monitorLoop() {
	ticker := time.NewTicker(tm.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopCh:
			return
		case <-ticker.C:
			tm.checkThresholds()
		}
	}
}

func (tm *ThresholdMonitor) checkThresholds() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if tm.metricsSource == nil {
		return
	}

	// Check error rate
	errorRate := tm.metricsSource.GetErrorRate()
	if errorRate > tm.config.ErrorRateThreshold {
		tm.alertManager.Send(ctx, Alert{
			Name:     "high_error_rate",
			Severity: SeverityError,
			Message:  fmt.Sprintf("Error rate is %.2f%%, threshold is %.2f%%", errorRate, tm.config.ErrorRateThreshold),
			Tags: map[string]string{
				"metric": "error_rate",
			},
			Metadata: map[string]interface{}{
				"current_value": errorRate,
				"threshold":     tm.config.ErrorRateThreshold,
			},
		})
	}

	// Check latency
	avgLatency := tm.metricsSource.GetAverageLatencyMs()
	if avgLatency > tm.config.LatencyThresholdMs {
		tm.alertManager.Send(ctx, Alert{
			Name:     "high_latency",
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("Average latency is %.2fms, threshold is %.2fms", avgLatency, tm.config.LatencyThresholdMs),
			Tags: map[string]string{
				"metric": "latency",
			},
			Metadata: map[string]interface{}{
				"current_value": avgLatency,
				"threshold":     tm.config.LatencyThresholdMs,
			},
		})
	}

	// Check circuit breaker
	if tm.config.CircuitBreakerAlert && tm.metricsSource.IsCircuitBreakerOpen() {
		tm.alertManager.Send(ctx, Alert{
			Name:     "circuit_breaker_open",
			Severity: SeverityCritical,
			Message:  "Circuit breaker is OPEN - IDR service may be unavailable",
			Tags: map[string]string{
				"component": "idr",
			},
		})
	}

	// Check rate limit rejections (calculate rate per minute)
	now := time.Now()
	currentRejects := tm.metricsSource.GetRateLimitRejections()
	elapsed := now.Sub(tm.lastCheck)

	if elapsed > 0 && tm.lastRateLimitRejects > 0 {
		rejectsPerMinute := float64(currentRejects-tm.lastRateLimitRejects) / elapsed.Minutes()
		if int(rejectsPerMinute) > tm.config.RateLimitThreshold {
			tm.alertManager.Send(ctx, Alert{
				Name:     "high_rate_limit_rejections",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Rate limit rejections: %.0f/min, threshold: %d/min", rejectsPerMinute, tm.config.RateLimitThreshold),
				Tags: map[string]string{
					"metric": "rate_limit",
				},
				Metadata: map[string]interface{}{
					"rejections_per_minute": rejectsPerMinute,
					"threshold":             tm.config.RateLimitThreshold,
				},
			})
		}
	}

	tm.lastCheck = now
	tm.lastRateLimitRejects = currentRejects
	tm.lastTotalRequests = tm.metricsSource.GetTotalRequests()
}

// CheckNow runs an immediate threshold check (useful for testing)
func (tm *ThresholdMonitor) CheckNow() {
	tm.checkThresholds()
}

func parseFloatEnv(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func parseIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
