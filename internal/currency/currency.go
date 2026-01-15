// Package currency provides currency conversion for bid normalization
package currency

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// Converter handles currency conversion with cached exchange rates
type Converter struct {
	mu           sync.RWMutex
	rates        map[string]float64 // Rates relative to base currency
	baseCurrency string
	lastUpdate   time.Time
	provider     RateProvider
	config       *Config
}

// Config holds currency converter configuration
type Config struct {
	// Enabled controls whether conversion is active
	Enabled bool `json:"enabled"`

	// BaseCurrency is the currency rates are relative to (default: USD)
	BaseCurrency string `json:"base_currency"`

	// RefreshInterval for rate updates
	RefreshInterval time.Duration `json:"refresh_interval"`

	// FetchTimeout for rate provider requests
	FetchTimeout time.Duration `json:"fetch_timeout"`

	// StaleRateThreshold - rates older than this trigger a warning
	StaleRateThreshold time.Duration `json:"stale_rate_threshold"`

	// DefaultRates fallback rates if provider fails
	DefaultRates map[string]float64 `json:"default_rates"`
}

// DefaultConfig returns production-safe defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:            true,
		BaseCurrency:       "USD",
		RefreshInterval:    1 * time.Hour,
		FetchTimeout:       5 * time.Second,
		StaleRateThreshold: 24 * time.Hour,
		DefaultRates: map[string]float64{
			"USD": 1.0,
			"EUR": 0.92,
			"GBP": 0.79,
			"JPY": 149.50,
			"CAD": 1.36,
			"AUD": 1.53,
			"CHF": 0.88,
			"CNY": 7.24,
			"INR": 83.12,
			"MXN": 17.15,
			"BRL": 4.97,
			"KRW": 1325.0,
			"SGD": 1.34,
			"HKD": 7.82,
			"SEK": 10.42,
			"NOK": 10.65,
			"DKK": 6.87,
			"PLN": 3.98,
			"RUB": 92.50,
			"ZAR": 18.75,
		},
	}
}

// RateProvider is the interface for exchange rate sources
type RateProvider interface {
	// FetchRates returns exchange rates relative to base currency
	FetchRates(ctx context.Context, baseCurrency string) (map[string]float64, error)

	// Name returns the provider name for logging
	Name() string
}

// NewConverter creates a new currency converter
func NewConverter(config *Config, provider RateProvider) *Converter {
	if config == nil {
		config = DefaultConfig()
	}

	c := &Converter{
		rates:        make(map[string]float64),
		baseCurrency: config.BaseCurrency,
		provider:     provider,
		config:       config,
	}

	// Initialize with default rates
	if config.DefaultRates != nil {
		for currency, rate := range config.DefaultRates {
			c.rates[currency] = rate
		}
	}

	return c
}

// Convert converts an amount from one currency to another
// Returns the converted amount and any error
func (c *Converter) Convert(amount float64, from, to string) (float64, error) {
	if !c.config.Enabled {
		return amount, nil
	}

	// Same currency - no conversion needed
	if from == to {
		return amount, nil
	}

	// Normalize currency codes
	from = normalizeCurrency(from)
	to = normalizeCurrency(to)

	c.mu.RLock()
	fromRate, fromOK := c.rates[from]
	toRate, toOK := c.rates[to]
	c.mu.RUnlock()

	if !fromOK {
		return 0, fmt.Errorf("unknown source currency: %s", from)
	}
	if !toOK {
		return 0, fmt.Errorf("unknown target currency: %s", to)
	}

	// Convert: amount in 'from' -> base currency -> 'to' currency
	// If rates are relative to USD:
	// amount_usd = amount / fromRate
	// amount_to = amount_usd * toRate
	converted := (amount / fromRate) * toRate

	return converted, nil
}

// ConvertBidPrice converts a bid price to the request currency
// This is the main entry point for auction bid normalization
func (c *Converter) ConvertBidPrice(bidPrice float64, bidCurrency, requestCurrency string) (float64, error) {
	if bidCurrency == "" {
		bidCurrency = "USD" // Default bid currency
	}
	if requestCurrency == "" {
		requestCurrency = "USD" // Default request currency
	}

	return c.Convert(bidPrice, bidCurrency, requestCurrency)
}

// GetRate returns the exchange rate for a currency relative to base
func (c *Converter) GetRate(currency string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	currency = normalizeCurrency(currency)
	rate, ok := c.rates[currency]
	return rate, ok
}

// GetRates returns a copy of all current rates
func (c *Converter) GetRates() map[string]float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rates := make(map[string]float64, len(c.rates))
	for k, v := range c.rates {
		rates[k] = v
	}
	return rates
}

// RefreshRates fetches fresh rates from the provider
func (c *Converter) RefreshRates(ctx context.Context) error {
	if c.provider == nil {
		return fmt.Errorf("no rate provider configured")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, c.config.FetchTimeout)
	defer cancel()

	rates, err := c.provider.FetchRates(fetchCtx, c.baseCurrency)
	if err != nil {
		logger.Log.Warn().
			Err(err).
			Str("provider", c.provider.Name()).
			Msg("Failed to fetch currency rates")
		return err
	}

	c.mu.Lock()
	for currency, rate := range rates {
		c.rates[normalizeCurrency(currency)] = rate
	}
	c.lastUpdate = time.Now()
	c.mu.Unlock()

	logger.Log.Info().
		Int("currencies", len(rates)).
		Str("provider", c.provider.Name()).
		Msg("Currency rates updated")

	return nil
}

// StartAutoRefresh starts a background goroutine that refreshes rates periodically
func (c *Converter) StartAutoRefresh(ctx context.Context) {
	go func() {
		// Initial fetch
		if err := c.RefreshRates(ctx); err != nil {
			logger.Log.Warn().Err(err).Msg("Initial currency rate fetch failed, using defaults")
		}

		ticker := time.NewTicker(c.config.RefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.RefreshRates(ctx); err != nil {
					logger.Log.Warn().Err(err).Msg("Currency rate refresh failed")
				}
			}
		}
	}()
}

// SetRate manually sets a rate (useful for testing or overrides)
func (c *Converter) SetRate(currency string, rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rates[normalizeCurrency(currency)] = rate
}

// SetRates bulk sets rates
func (c *Converter) SetRates(rates map[string]float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for currency, rate := range rates {
		c.rates[normalizeCurrency(currency)] = rate
	}
	c.lastUpdate = time.Now()
}

// LastUpdate returns the timestamp of the last rate update
func (c *Converter) LastUpdate() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUpdate
}

// IsStale returns true if rates are older than the stale threshold
func (c *Converter) IsStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastUpdate.IsZero() {
		return true
	}
	return time.Since(c.lastUpdate) > c.config.StaleRateThreshold
}

// GetConfig returns current configuration
func (c *Converter) GetConfig() *Config {
	return c.config
}

// normalizeCurrency normalizes currency codes to uppercase
func normalizeCurrency(code string) string {
	if len(code) != 3 {
		return code
	}
	// Manual uppercase without strings import
	result := make([]byte, 3)
	for i := 0; i < 3; i++ {
		if code[i] >= 'a' && code[i] <= 'z' {
			result[i] = code[i] - 32
		} else {
			result[i] = code[i]
		}
	}
	return string(result)
}

// ECBProvider fetches rates from the European Central Bank
type ECBProvider struct {
	httpClient *http.Client
	endpoint   string
}

// ECB XML response structures
type ecbEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Cube    ecbCube  `xml:"Cube>Cube"`
}

type ecbCube struct {
	Time  string       `xml:"time,attr"`
	Rates []ecbRate    `xml:"Cube"`
}

type ecbRate struct {
	Currency string  `xml:"currency,attr"`
	Rate     float64 `xml:"rate,attr"`
}

// NewECBProvider creates a provider that fetches from ECB
func NewECBProvider() *ECBProvider {
	return &ECBProvider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		endpoint:   "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml",
	}
}

// Name returns the provider name
func (p *ECBProvider) Name() string {
	return "ecb"
}

// FetchRates fetches current rates from ECB
// ECB provides rates relative to EUR, so we convert to the requested base
func (p *ECBProvider) FetchRates(ctx context.Context, baseCurrency string) (map[string]float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ECB rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ECB returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var envelope ecbEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse ECB XML: %w", err)
	}

	// ECB rates are relative to EUR
	eurRates := make(map[string]float64)
	eurRates["EUR"] = 1.0

	for _, rate := range envelope.Cube.Rates {
		eurRates[rate.Currency] = rate.Rate
	}

	// If base currency is EUR, return as-is
	if baseCurrency == "EUR" {
		return eurRates, nil
	}

	// Convert to requested base currency
	baseRate, ok := eurRates[baseCurrency]
	if !ok {
		return nil, fmt.Errorf("base currency %s not found in ECB rates", baseCurrency)
	}

	// Convert all rates to be relative to base currency
	rates := make(map[string]float64)
	for currency, eurRate := range eurRates {
		// rate relative to base = eurRate / baseRate
		rates[currency] = eurRate / baseRate
	}

	return rates, nil
}

// APIProvider fetches rates from a custom API endpoint
type APIProvider struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
}

// APIProviderConfig holds API provider configuration
type APIProviderConfig struct {
	// Endpoint URL (supports {{base}} template variable)
	Endpoint string `json:"endpoint"`

	// APIKey for authentication
	APIKey string `json:"api_key"`

	// Timeout for requests
	Timeout time.Duration `json:"timeout"`
}

// NewAPIProvider creates a custom API rate provider
func NewAPIProvider(config *APIProviderConfig) *APIProvider {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &APIProvider{
		httpClient: &http.Client{Timeout: timeout},
		endpoint:   config.Endpoint,
		apiKey:     config.APIKey,
	}
}

// Name returns the provider name
func (p *APIProvider) Name() string {
	return "api"
}

// FetchRates fetches rates from the custom API
// Expects JSON response: {"rates": {"USD": 1.0, "EUR": 0.92, ...}}
func (p *APIProvider) FetchRates(ctx context.Context, baseCurrency string) (map[string]float64, error) {
	url := p.endpoint
	// Replace template variables
	for i := 0; i < len(url)-7; i++ {
		if url[i:i+8] == "{{base}}" {
			url = url[:i] + baseCurrency + url[i+8:]
			break
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		req.Header.Set("X-API-Key", p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Rates map[string]float64 `json:"rates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Rates, nil
}

// StaticProvider provides fixed rates (useful for testing)
type StaticProvider struct {
	rates map[string]float64
}

// NewStaticProvider creates a provider with fixed rates
func NewStaticProvider(rates map[string]float64) *StaticProvider {
	return &StaticProvider{rates: rates}
}

// Name returns the provider name
func (p *StaticProvider) Name() string {
	return "static"
}

// FetchRates returns the static rates
func (p *StaticProvider) FetchRates(ctx context.Context, baseCurrency string) (map[string]float64, error) {
	return p.rates, nil
}

// SetRates updates the static rates
func (p *StaticProvider) SetRates(rates map[string]float64) {
	p.rates = rates
}
