// Package alerting provides webhook-based alerting for monitoring and incident response
package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// Severity represents alert severity levels
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityError    Severity = "error"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Alert represents an alert to be sent
type Alert struct {
	Name        string                 `json:"name"`
	Severity    Severity               `json:"severity"`
	Message     string                 `json:"message"`
	Description string                 `json:"description,omitempty"`
	Source      string                 `json:"source"`
	Timestamp   time.Time              `json:"timestamp"`
	Tags        map[string]string      `json:"tags,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// WebhookType represents the type of webhook destination
type WebhookType string

const (
	WebhookSlack     WebhookType = "slack"
	WebhookDiscord   WebhookType = "discord"
	WebhookPagerDuty WebhookType = "pagerduty"
	WebhookGeneric   WebhookType = "generic"
)

// WebhookConfig holds configuration for a webhook destination
type WebhookConfig struct {
	Type       WebhookType `json:"type"`
	URL        string      `json:"url"`
	Enabled    bool        `json:"enabled"`
	MinSeverity Severity   `json:"min_severity"` // Only send alerts >= this severity
}

// Config holds alerting configuration
type Config struct {
	Enabled         bool            `json:"enabled"`
	ServiceName     string          `json:"service_name"`
	Environment     string          `json:"environment"`
	Webhooks        []WebhookConfig `json:"webhooks"`
	RateLimitWindow time.Duration   `json:"rate_limit_window"` // Dedupe window
	HTTPTimeout     time.Duration   `json:"http_timeout"`
}

// DefaultConfig returns sensible defaults for alerting configuration
func DefaultConfig() Config {
	cfg := Config{
		Enabled:         false,
		ServiceName:     getEnvOrDefault("ALERT_SERVICE_NAME", "pbs"),
		Environment:     getEnvOrDefault("ALERT_ENVIRONMENT", "development"),
		Webhooks:        []WebhookConfig{},
		RateLimitWindow: 5 * time.Minute,
		HTTPTimeout:     10 * time.Second,
	}

	// Configure Slack webhook if set
	if slackURL := os.Getenv("ALERT_SLACK_WEBHOOK_URL"); slackURL != "" {
		cfg.Enabled = true
		cfg.Webhooks = append(cfg.Webhooks, WebhookConfig{
			Type:        WebhookSlack,
			URL:         slackURL,
			Enabled:     true,
			MinSeverity: SeverityWarning,
		})
	}

	// Configure Discord webhook if set
	if discordURL := os.Getenv("ALERT_DISCORD_WEBHOOK_URL"); discordURL != "" {
		cfg.Enabled = true
		cfg.Webhooks = append(cfg.Webhooks, WebhookConfig{
			Type:        WebhookDiscord,
			URL:         discordURL,
			Enabled:     true,
			MinSeverity: SeverityWarning,
		})
	}

	// Configure PagerDuty if set
	if pdKey := os.Getenv("ALERT_PAGERDUTY_ROUTING_KEY"); pdKey != "" {
		cfg.Enabled = true
		cfg.Webhooks = append(cfg.Webhooks, WebhookConfig{
			Type:        WebhookPagerDuty,
			URL:         "https://events.pagerduty.com/v2/enqueue",
			Enabled:     true,
			MinSeverity: SeverityCritical, // Only page on critical
		})
	}

	// Configure generic webhook if set
	if genericURL := os.Getenv("ALERT_WEBHOOK_URL"); genericURL != "" {
		cfg.Enabled = true
		cfg.Webhooks = append(cfg.Webhooks, WebhookConfig{
			Type:        WebhookGeneric,
			URL:         genericURL,
			Enabled:     true,
			MinSeverity: SeverityWarning,
		})
	}

	return cfg
}

// Manager handles sending alerts to configured webhooks
type Manager struct {
	config     Config
	httpClient *http.Client
	mu         sync.Mutex
	recentAlerts map[string]time.Time // For deduplication
	pdRoutingKey string
}

// NewManager creates a new alert manager
func NewManager(cfg Config) *Manager {
	return &Manager{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
		recentAlerts: make(map[string]time.Time),
		pdRoutingKey: os.Getenv("ALERT_PAGERDUTY_ROUTING_KEY"),
	}
}

// IsEnabled returns true if alerting is enabled
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled && len(m.config.Webhooks) > 0
}

// Send sends an alert to all configured webhooks
func (m *Manager) Send(ctx context.Context, alert Alert) error {
	if !m.IsEnabled() {
		return nil
	}

	// Add defaults
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}
	if alert.Source == "" {
		alert.Source = m.config.ServiceName
	}
	if alert.Tags == nil {
		alert.Tags = make(map[string]string)
	}
	alert.Tags["environment"] = m.config.Environment

	// Check rate limiting
	if m.isRateLimited(alert) {
		return nil
	}

	var errs []error
	for _, webhook := range m.config.Webhooks {
		if !webhook.Enabled {
			continue
		}
		if !m.shouldSend(alert.Severity, webhook.MinSeverity) {
			continue
		}

		var err error
		switch webhook.Type {
		case WebhookSlack:
			err = m.sendSlack(ctx, webhook.URL, alert)
		case WebhookDiscord:
			err = m.sendDiscord(ctx, webhook.URL, alert)
		case WebhookPagerDuty:
			err = m.sendPagerDuty(ctx, alert)
		case WebhookGeneric:
			err = m.sendGeneric(ctx, webhook.URL, alert)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", webhook.Type, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to send alerts: %v", errs)
	}
	return nil
}

// shouldSend returns true if alert severity >= minimum severity
func (m *Manager) shouldSend(alertSeverity, minSeverity Severity) bool {
	severityOrder := map[Severity]int{
		SeverityInfo:     0,
		SeverityWarning:  1,
		SeverityError:    2,
		SeverityCritical: 3,
	}
	return severityOrder[alertSeverity] >= severityOrder[minSeverity]
}

// isRateLimited checks if this alert was sent recently
func (m *Manager) isRateLimited(alert Alert) bool {
	key := fmt.Sprintf("%s:%s:%s", alert.Name, alert.Severity, alert.Message)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean old entries
	now := time.Now()
	for k, t := range m.recentAlerts {
		if now.Sub(t) > m.config.RateLimitWindow {
			delete(m.recentAlerts, k)
		}
	}

	if lastSent, exists := m.recentAlerts[key]; exists {
		if now.Sub(lastSent) < m.config.RateLimitWindow {
			return true
		}
	}

	m.recentAlerts[key] = now
	return false
}

// sendSlack sends an alert to Slack
func (m *Manager) sendSlack(ctx context.Context, url string, alert Alert) error {
	color := m.severityColor(alert.Severity)

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  fmt.Sprintf("[%s] %s", alert.Severity, alert.Name),
				"text":   alert.Message,
				"footer": fmt.Sprintf("%s | %s", alert.Source, m.config.Environment),
				"ts":     alert.Timestamp.Unix(),
				"fields": m.buildSlackFields(alert),
			},
		},
	}

	return m.postJSON(ctx, url, payload)
}

// buildSlackFields converts alert metadata to Slack fields
func (m *Manager) buildSlackFields(alert Alert) []map[string]interface{} {
	var fields []map[string]interface{}

	for k, v := range alert.Tags {
		fields = append(fields, map[string]interface{}{
			"title": k,
			"value": v,
			"short": true,
		})
	}

	if alert.Description != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Description",
			"value": alert.Description,
			"short": false,
		})
	}

	return fields
}

// sendDiscord sends an alert to Discord
func (m *Manager) sendDiscord(ctx context.Context, url string, alert Alert) error {
	color := m.severityColorInt(alert.Severity)

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("[%s] %s", alert.Severity, alert.Name),
				"description": alert.Message,
				"color":       color,
				"footer": map[string]string{
					"text": fmt.Sprintf("%s | %s", alert.Source, m.config.Environment),
				},
				"timestamp": alert.Timestamp.Format(time.RFC3339),
			},
		},
	}

	return m.postJSON(ctx, url, payload)
}

// sendPagerDuty sends an alert to PagerDuty Events API v2
func (m *Manager) sendPagerDuty(ctx context.Context, alert Alert) error {
	if m.pdRoutingKey == "" {
		return fmt.Errorf("PagerDuty routing key not configured")
	}

	severity := "warning"
	switch alert.Severity {
	case SeverityCritical:
		severity = "critical"
	case SeverityError:
		severity = "error"
	case SeverityWarning:
		severity = "warning"
	case SeverityInfo:
		severity = "info"
	}

	payload := map[string]interface{}{
		"routing_key":  m.pdRoutingKey,
		"event_action": "trigger",
		"dedup_key":    fmt.Sprintf("%s-%s-%s", m.config.ServiceName, alert.Name, alert.Severity),
		"payload": map[string]interface{}{
			"summary":   fmt.Sprintf("[%s] %s: %s", m.config.Environment, alert.Name, alert.Message),
			"source":    alert.Source,
			"severity":  severity,
			"timestamp": alert.Timestamp.Format(time.RFC3339),
			"custom_details": map[string]interface{}{
				"environment": m.config.Environment,
				"tags":        alert.Tags,
				"metadata":    alert.Metadata,
			},
		},
	}

	return m.postJSON(ctx, "https://events.pagerduty.com/v2/enqueue", payload)
}

// sendGeneric sends an alert to a generic webhook endpoint
func (m *Manager) sendGeneric(ctx context.Context, url string, alert Alert) error {
	payload := map[string]interface{}{
		"alert":       alert,
		"service":     m.config.ServiceName,
		"environment": m.config.Environment,
	}
	return m.postJSON(ctx, url, payload)
}

// postJSON sends a JSON POST request
func (m *Manager) postJSON(ctx context.Context, url string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// severityColor returns a hex color for Slack
func (m *Manager) severityColor(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "#dc3545" // Red
	case SeverityError:
		return "#fd7e14" // Orange
	case SeverityWarning:
		return "#ffc107" // Yellow
	case SeverityInfo:
		return "#17a2b8" // Blue
	default:
		return "#6c757d" // Gray
	}
}

// severityColorInt returns an integer color for Discord
func (m *Manager) severityColorInt(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return 0xdc3545 // Red
	case SeverityError:
		return 0xfd7e14 // Orange
	case SeverityWarning:
		return 0xffc107 // Yellow
	case SeverityInfo:
		return 0x17a2b8 // Blue
	default:
		return 0x6c757d // Gray
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// Convenience functions for common alerts

// Critical sends a critical severity alert
func (m *Manager) Critical(ctx context.Context, name, message string) error {
	return m.Send(ctx, Alert{
		Name:     name,
		Severity: SeverityCritical,
		Message:  message,
	})
}

// Error sends an error severity alert
func (m *Manager) Error(ctx context.Context, name, message string) error {
	return m.Send(ctx, Alert{
		Name:     name,
		Severity: SeverityError,
		Message:  message,
	})
}

// Warning sends a warning severity alert
func (m *Manager) Warning(ctx context.Context, name, message string) error {
	return m.Send(ctx, Alert{
		Name:     name,
		Severity: SeverityWarning,
		Message:  message,
	})
}

// Info sends an info severity alert
func (m *Manager) Info(ctx context.Context, name, message string) error {
	return m.Send(ctx, Alert{
		Name:     name,
		Severity: SeverityInfo,
		Message:  message,
	})
}

// ErrorWithDetails sends an error alert with additional context
func (m *Manager) ErrorWithDetails(ctx context.Context, name, message string, err error, metadata map[string]interface{}) error {
	alert := Alert{
		Name:     name,
		Severity: SeverityError,
		Message:  message,
		Metadata: metadata,
	}
	if err != nil {
		if alert.Metadata == nil {
			alert.Metadata = make(map[string]interface{})
		}
		alert.Metadata["error"] = err.Error()
	}
	return m.Send(ctx, alert)
}
