package currency

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConverter_Convert(t *testing.T) {
	config := DefaultConfig()
	converter := NewConverter(config, nil)

	// Set known rates for testing
	converter.SetRates(map[string]float64{
		"USD": 1.0,
		"EUR": 0.85,
		"GBP": 0.75,
		"JPY": 110.0,
	})

	tests := []struct {
		name     string
		amount   float64
		from     string
		to       string
		expected float64
		wantErr  bool
	}{
		{
			name:     "same currency",
			amount:   100.0,
			from:     "USD",
			to:       "USD",
			expected: 100.0,
		},
		{
			name:     "USD to EUR",
			amount:   100.0,
			from:     "USD",
			to:       "EUR",
			expected: 85.0, // 100 * 0.85
		},
		{
			name:     "EUR to USD",
			amount:   85.0,
			from:     "EUR",
			to:       "USD",
			expected: 100.0, // 85 / 0.85
		},
		{
			name:     "EUR to GBP",
			amount:   100.0,
			from:     "EUR",
			to:       "GBP",
			expected: 88.235294, // (100 / 0.85) * 0.75
		},
		{
			name:     "USD to JPY",
			amount:   100.0,
			from:     "USD",
			to:       "JPY",
			expected: 11000.0, // 100 * 110
		},
		{
			name:     "lowercase currency codes",
			amount:   100.0,
			from:     "usd",
			to:       "eur",
			expected: 85.0,
		},
		{
			name:    "unknown source currency",
			amount:  100.0,
			from:    "XYZ",
			to:      "USD",
			wantErr: true,
		},
		{
			name:    "unknown target currency",
			amount:  100.0,
			from:    "USD",
			to:      "XYZ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := converter.Convert(tt.amount, tt.from, tt.to)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Allow small floating point differences
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.001 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestConverter_ConvertBidPrice(t *testing.T) {
	config := DefaultConfig()
	converter := NewConverter(config, nil)
	converter.SetRates(map[string]float64{
		"USD": 1.0,
		"EUR": 0.92,
	})

	// Test with explicit currencies
	result, err := converter.ConvertBidPrice(1.0, "EUR", "USD")
	if err != nil {
		t.Fatal(err)
	}
	expected := 1.0 / 0.92 // EUR to USD
	diff := result - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.001 {
		t.Errorf("expected %f, got %f", expected, result)
	}

	// Test with empty currencies (should default to USD)
	result, err = converter.ConvertBidPrice(1.0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if result != 1.0 {
		t.Errorf("expected 1.0 for same currency, got %f", result)
	}
}

func TestConverter_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	converter := NewConverter(config, nil)

	// When disabled, should return original amount
	result, err := converter.Convert(100.0, "EUR", "USD")
	if err != nil {
		t.Fatal(err)
	}
	if result != 100.0 {
		t.Errorf("expected original amount when disabled, got %f", result)
	}
}

func TestConverter_GetRate(t *testing.T) {
	converter := NewConverter(DefaultConfig(), nil)
	converter.SetRate("EUR", 0.92)

	rate, ok := converter.GetRate("EUR")
	if !ok {
		t.Fatal("expected rate to exist")
	}
	if rate != 0.92 {
		t.Errorf("expected 0.92, got %f", rate)
	}

	// Test lowercase
	rate, ok = converter.GetRate("eur")
	if !ok {
		t.Fatal("expected rate to exist for lowercase")
	}
	if rate != 0.92 {
		t.Errorf("expected 0.92, got %f", rate)
	}

	// Test unknown currency
	_, ok = converter.GetRate("XYZ")
	if ok {
		t.Error("expected false for unknown currency")
	}
}

func TestConverter_GetRates(t *testing.T) {
	converter := NewConverter(DefaultConfig(), nil)
	converter.SetRates(map[string]float64{
		"USD": 1.0,
		"EUR": 0.92,
		"GBP": 0.79,
	})

	rates := converter.GetRates()

	if len(rates) < 3 {
		t.Errorf("expected at least 3 rates, got %d", len(rates))
	}

	// Verify it's a copy (modifications don't affect original)
	rates["USD"] = 999.0
	rate, _ := converter.GetRate("USD")
	if rate == 999.0 {
		t.Error("GetRates should return a copy")
	}
}

func TestConverter_IsStale(t *testing.T) {
	config := DefaultConfig()
	config.StaleRateThreshold = 100 * time.Millisecond
	converter := NewConverter(config, nil)

	// Initially stale (no update)
	if !converter.IsStale() {
		t.Error("expected stale initially")
	}

	// After setting rates, not stale
	converter.SetRates(map[string]float64{"USD": 1.0})
	if converter.IsStale() {
		t.Error("expected not stale after update")
	}

	// Wait for threshold
	time.Sleep(150 * time.Millisecond)
	if !converter.IsStale() {
		t.Error("expected stale after threshold")
	}
}

func TestStaticProvider(t *testing.T) {
	rates := map[string]float64{
		"USD": 1.0,
		"EUR": 0.92,
	}

	provider := NewStaticProvider(rates)

	if provider.Name() != "static" {
		t.Errorf("expected name 'static', got '%s'", provider.Name())
	}

	fetched, err := provider.FetchRates(context.Background(), "USD")
	if err != nil {
		t.Fatal(err)
	}

	if fetched["EUR"] != 0.92 {
		t.Errorf("expected EUR rate 0.92, got %f", fetched["EUR"])
	}
}

func TestAPIProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key header
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("expected API key header")
		}

		response := map[string]interface{}{
			"rates": map[string]float64{
				"USD": 1.0,
				"EUR": 0.92,
				"GBP": 0.79,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewAPIProvider(&APIProviderConfig{
		Endpoint: server.URL + "/rates?base={{base}}",
		APIKey:   "test-key",
		Timeout:  1 * time.Second,
	})

	if provider.Name() != "api" {
		t.Errorf("expected name 'api', got '%s'", provider.Name())
	}

	rates, err := provider.FetchRates(context.Background(), "USD")
	if err != nil {
		t.Fatal(err)
	}

	if rates["EUR"] != 0.92 {
		t.Errorf("expected EUR rate 0.92, got %f", rates["EUR"])
	}
}

func TestConverter_RefreshRates(t *testing.T) {
	provider := NewStaticProvider(map[string]float64{
		"USD": 1.0,
		"EUR": 0.90,
	})

	converter := NewConverter(DefaultConfig(), provider)

	// Initial rates from default config
	rate, _ := converter.GetRate("EUR")
	if rate != 0.92 { // From DefaultConfig
		t.Errorf("expected initial rate 0.92, got %f", rate)
	}

	// Refresh from provider
	err := converter.RefreshRates(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should now have provider's rate
	rate, _ = converter.GetRate("EUR")
	if rate != 0.90 {
		t.Errorf("expected refreshed rate 0.90, got %f", rate)
	}
}

func TestConverter_RefreshRates_NoProvider(t *testing.T) {
	converter := NewConverter(DefaultConfig(), nil)

	err := converter.RefreshRates(context.Background())
	if err == nil {
		t.Error("expected error with no provider")
	}
}

func TestNormalizeCurrency(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"usd", "USD"},
		{"USD", "USD"},
		{"Eur", "EUR"},
		{"gbp", "GBP"},
		{"US", "US"}, // Not 3 chars, unchanged
		{"USDD", "USDD"}, // Not 3 chars, unchanged
	}

	for _, tt := range tests {
		result := normalizeCurrency(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeCurrency(%s) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("expected enabled by default")
	}

	if config.BaseCurrency != "USD" {
		t.Errorf("expected USD base, got %s", config.BaseCurrency)
	}

	if len(config.DefaultRates) < 10 {
		t.Errorf("expected at least 10 default rates, got %d", len(config.DefaultRates))
	}

	// Check that USD rate is 1.0
	if config.DefaultRates["USD"] != 1.0 {
		t.Errorf("expected USD rate 1.0, got %f", config.DefaultRates["USD"])
	}
}

func TestECBProvider_Name(t *testing.T) {
	provider := NewECBProvider()
	if provider.Name() != "ecb" {
		t.Errorf("expected name 'ecb', got '%s'", provider.Name())
	}
}
