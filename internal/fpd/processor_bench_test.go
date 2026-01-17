package fpd

import (
	"testing"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// ============================================================================
// FPD PROCESSING BENCHMARKS
// ============================================================================

// BenchmarkProcessor_ProcessRequest_Simple benchmarks basic FPD processing
func BenchmarkProcessor_ProcessRequest_Simple(b *testing.B) {
	config := &Config{
		Enabled: true,
	}
	proc := NewProcessor(config)

	req := &openrtb.BidRequest{
		ID: "test-req",
		Site: &openrtb.Site{
			ID:     "site1",
			Domain: "example.com",
		},
		Imp: []openrtb.Imp{
			{ID: "imp1"},
		},
	}

	bidders := []string{"rubicon", "appnexus", "pubmatic"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = proc.ProcessRequest(req, bidders)
	}
}

// BenchmarkProcessor_ProcessRequest_WithFPD benchmarks FPD processing with actual FPD data
func BenchmarkProcessor_ProcessRequest_WithFPD(b *testing.B) {
	config := &Config{
		Enabled: true,
	}
	proc := NewProcessor(config)

	req := &openrtb.BidRequest{
		ID: "test-req",
		Site: &openrtb.Site{
			ID:     "site1",
			Domain: "example.com",
		},
		Imp: []openrtb.Imp{
			{ID: "imp1"},
		},
	}

	bidders := []string{"rubicon", "appnexus", "pubmatic", "openx"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = proc.ProcessRequest(req, bidders)
	}
}

// BenchmarkProcessor_ProcessRequest_MultipleBidders benchmarks FPD with many bidders
func BenchmarkProcessor_ProcessRequest_MultipleBidders(b *testing.B) {
	config := &Config{
		Enabled: true,
	}
	proc := NewProcessor(config)

	req := &openrtb.BidRequest{
		ID: "test-req",
		Site: &openrtb.Site{
			ID:     "site1",
			Domain: "example.com",
		},
		Imp: []openrtb.Imp{
			{ID: "imp1"},
		},
	}

	bidders := []string{
		"rubicon", "appnexus", "pubmatic", "openx", "ix",
		"sovrn", "smartadserver", "mediagrid", "triplelift", "criteo",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = proc.ProcessRequest(req, bidders)
	}
}

// BenchmarkProcessor_ProcessRequest_Disabled benchmarks overhead when FPD is disabled
func BenchmarkProcessor_ProcessRequest_Disabled(b *testing.B) {
	config := &Config{
		Enabled: false,
	}
	proc := NewProcessor(config)

	req := &openrtb.BidRequest{
		ID:   "test-req",
		Site: &openrtb.Site{ID: "site1"},
		Imp:  []openrtb.Imp{{ID: "imp1"}},
	}

	bidders := []string{"rubicon", "appnexus"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = proc.ProcessRequest(req, bidders)
	}
}

// BenchmarkProcessor_UpdateConfig benchmarks atomic config updates
func BenchmarkProcessor_UpdateConfig(b *testing.B) {
	proc := NewProcessor(DefaultConfig())

	newConfig := &Config{
		Enabled: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proc.UpdateConfig(newConfig)
	}
}

// BenchmarkProcessor_GetConfig benchmarks atomic config reads
func BenchmarkProcessor_GetConfig(b *testing.B) {
	proc := NewProcessor(DefaultConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = proc.getConfig()
	}
}
