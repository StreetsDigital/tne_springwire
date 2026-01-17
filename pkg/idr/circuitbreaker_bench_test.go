package idr

import (
	"sync"
	"testing"
	"time"
)

// ============================================================================
// CIRCUIT BREAKER OVERHEAD BENCHMARKS
// ============================================================================

// BenchmarkCircuitBreaker_Execute_Closed benchmarks Execute() when circuit is closed
func BenchmarkCircuitBreaker_Execute_Closed(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		MaxConcurrent:    100,
	})

	fn := func() error {
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Execute(fn)
	}
}

// BenchmarkCircuitBreaker_Execute_Open benchmarks Execute() when circuit is open (fast-fail)
func BenchmarkCircuitBreaker_Execute_Open(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		MaxConcurrent:    100,
	})

	// Force circuit open
	cb.ForceOpen()

	fn := func() error {
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Execute(fn)
	}
}

// BenchmarkCircuitBreaker_RecordFailure benchmarks direct failure recording
func BenchmarkCircuitBreaker_RecordFailure(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 1000,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.RecordFailure()
	}
}

// BenchmarkCircuitBreaker_RecordSuccess benchmarks direct success recording
func BenchmarkCircuitBreaker_RecordSuccess(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.RecordSuccess()
	}
}

// BenchmarkCircuitBreaker_IsOpen benchmarks state checking
func BenchmarkCircuitBreaker_IsOpen(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.IsOpen()
	}
}

// BenchmarkCircuitBreaker_Stats benchmarks stats retrieval
func BenchmarkCircuitBreaker_Stats(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	})

	// Record some activity
	for i := 0; i < 10; i++ {
		cb.RecordSuccess()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Stats()
	}
}

// BenchmarkCircuitBreaker_Concurrent benchmarks concurrent Execute() calls
func BenchmarkCircuitBreaker_Concurrent(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		MaxConcurrent:    100,
	})

	fn := func() error {
		return nil
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Execute(fn)
		}
	})
}

// BenchmarkCircuitBreaker_StateTransition benchmarks state change overhead
func BenchmarkCircuitBreaker_StateTransition(b *testing.B) {
	stateChangeCalls := 0
	var mu sync.Mutex

	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          1 * time.Nanosecond, // Allow immediate transition
		OnStateChange: func(from, to string) {
			mu.Lock()
			stateChangeCalls++
			mu.Unlock()
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Trigger state transitions
		for j := 0; j < 5; j++ {
			cb.RecordFailure()
		}
		cb.Reset()
	}
}

// BenchmarkCircuitBreaker_MaxConcurrent benchmarks concurrent limit enforcement
func BenchmarkCircuitBreaker_MaxConcurrent(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 1000,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		MaxConcurrent:    10, // Low limit
	})

	fn := func() error {
		time.Sleep(1 * time.Microsecond)
		return nil
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Execute(fn)
		}
	})
}

// BenchmarkCircuitBreaker_Comparison_NoCircuitBreaker benchmarks baseline without circuit breaker
func BenchmarkCircuitBreaker_Comparison_NoCircuitBreaker(b *testing.B) {
	fn := func() error {
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fn()
	}
}

// BenchmarkCircuitBreaker_Comparison_WithCircuitBreaker benchmarks overhead of circuit breaker
func BenchmarkCircuitBreaker_Comparison_WithCircuitBreaker(b *testing.B) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		MaxConcurrent:    100,
	})

	fn := func() error {
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Execute(fn)
	}
}


// ============================================================================
// EVENT RECORDER BENCHMARKS
// ============================================================================

// BenchmarkEventRecorder_RecordBidResponse benchmarks bid response recording
func BenchmarkEventRecorder_RecordBidResponse(b *testing.B) {
	recorder := NewEventRecorder("http://test.events.com", 1000)

	bidCPM := 1.50
	floorPrice := 0.50

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder.RecordBidResponse(
			"req123",
			"rubicon",
			50.0,
			true,
			&bidCPM,
			&floorPrice,
			"US",
			"desktop",
			"banner",
			"300x250",
			"pub456",
			false,
			false,
			"",
		)
	}
}

// BenchmarkEventRecorder_RecordBidResponse_Concurrent benchmarks concurrent bid response recording
func BenchmarkEventRecorder_RecordBidResponse_Concurrent(b *testing.B) {
	recorder := NewEventRecorder("http://test.events.com", 10000)

	bidCPM := 1.50
	floorPrice := 0.50

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			recorder.RecordBidResponse(
				"req123",
				"rubicon",
				50.0,
				true,
				&bidCPM,
				&floorPrice,
				"US",
				"desktop",
				"banner",
				"300x250",
				"pub456",
				false,
				false,
				"",
			)
		}
	})
}
