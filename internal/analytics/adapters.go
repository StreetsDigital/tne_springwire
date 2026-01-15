package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// FileAdapter writes analytics events to a file (JSONL format)
type FileAdapter struct {
	mu       sync.Mutex
	file     *os.File
	encoder  *json.Encoder
	filename string
}

// NewFileAdapter creates a file-based analytics adapter
func NewFileAdapter(filename string) (*FileAdapter, error) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open analytics file: %w", err)
	}

	return &FileAdapter{
		file:     file,
		encoder:  json.NewEncoder(file),
		filename: filename,
	}, nil
}

// Name returns the adapter name
func (a *FileAdapter) Name() string {
	return "file"
}

// LogAuctionEvent writes the event to the file
func (a *FileAdapter) LogAuctionEvent(ctx context.Context, event *AuctionEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.encoder.Encode(event)
}

// Close closes the file
func (a *FileAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// Rotate closes the current file and opens a new one
func (a *FileAdapter) Rotate() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file != nil {
		a.file.Close()
	}

	// Rename old file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	rotatedName := a.filename + "." + timestamp

	if err := os.Rename(a.filename, rotatedName); err != nil && !os.IsNotExist(err) {
		logger.Log.Warn().Err(err).Msg("Failed to rotate analytics file")
	}

	// Open new file
	file, err := os.OpenFile(a.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	a.file = file
	a.encoder = json.NewEncoder(file)
	return nil
}

// StdoutAdapter writes analytics events to stdout (for development)
type StdoutAdapter struct {
	encoder *json.Encoder
	pretty  bool
}

// NewStdoutAdapter creates a stdout analytics adapter
func NewStdoutAdapter(pretty bool) *StdoutAdapter {
	encoder := json.NewEncoder(os.Stdout)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return &StdoutAdapter{
		encoder: encoder,
		pretty:  pretty,
	}
}

// Name returns the adapter name
func (a *StdoutAdapter) Name() string {
	return "stdout"
}

// LogAuctionEvent writes the event to stdout
func (a *StdoutAdapter) LogAuctionEvent(ctx context.Context, event *AuctionEvent) error {
	return a.encoder.Encode(event)
}

// Close is a no-op for stdout
func (a *StdoutAdapter) Close() error {
	return nil
}

// HTTPAdapter sends analytics events to an HTTP endpoint
type HTTPAdapter struct {
	endpoint   string
	httpClient *http.Client
	apiKey     string
	batchSize  int
	flushInterval time.Duration

	mu      sync.Mutex
	batch   []*AuctionEvent
	done    chan struct{}
	closed  bool
}

// HTTPAdapterConfig holds HTTP adapter configuration
type HTTPAdapterConfig struct {
	// Endpoint URL for analytics API
	Endpoint string `json:"endpoint"`

	// APIKey for authentication
	APIKey string `json:"api_key"`

	// Timeout for HTTP requests
	Timeout time.Duration `json:"timeout"`

	// BatchSize - events are batched before sending
	BatchSize int `json:"batch_size"`

	// FlushInterval - max time before flushing batch
	FlushInterval time.Duration `json:"flush_interval"`
}

// DefaultHTTPAdapterConfig returns default HTTP adapter config
func DefaultHTTPAdapterConfig() *HTTPAdapterConfig {
	return &HTTPAdapterConfig{
		Timeout:       5 * time.Second,
		BatchSize:     100,
		FlushInterval: 10 * time.Second,
	}
}

// NewHTTPAdapter creates an HTTP-based analytics adapter
func NewHTTPAdapter(config *HTTPAdapterConfig) *HTTPAdapter {
	if config == nil {
		config = DefaultHTTPAdapterConfig()
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	batchSize := config.BatchSize
	if batchSize == 0 {
		batchSize = 100
	}

	flushInterval := config.FlushInterval
	if flushInterval == 0 {
		flushInterval = 10 * time.Second
	}

	a := &HTTPAdapter{
		endpoint:      config.Endpoint,
		apiKey:        config.APIKey,
		httpClient:    &http.Client{Timeout: timeout},
		batchSize:     batchSize,
		flushInterval: flushInterval,
		batch:         make([]*AuctionEvent, 0, batchSize),
		done:          make(chan struct{}),
	}

	// Start flush goroutine
	go a.flushLoop()

	return a
}

// Name returns the adapter name
func (a *HTTPAdapter) Name() string {
	return "http"
}

// LogAuctionEvent adds event to batch
func (a *HTTPAdapter) LogAuctionEvent(ctx context.Context, event *AuctionEvent) error {
	a.mu.Lock()
	a.batch = append(a.batch, event)
	shouldFlush := len(a.batch) >= a.batchSize
	a.mu.Unlock()

	if shouldFlush {
		go a.flush()
	}

	return nil
}

// Close flushes remaining events and stops the adapter
func (a *HTTPAdapter) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()

	close(a.done)
	return a.flush()
}

// flushLoop periodically flushes the batch
func (a *HTTPAdapter) flushLoop() {
	ticker := time.NewTicker(a.flushInterval)
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

// flush sends the current batch to the endpoint
func (a *HTTPAdapter) flush() error {
	a.mu.Lock()
	if len(a.batch) == 0 {
		a.mu.Unlock()
		return nil
	}
	batch := a.batch
	a.batch = make([]*AuctionEvent, 0, a.batchSize)
	a.mu.Unlock()

	// Send batch
	return a.sendBatch(batch)
}

// sendBatch sends a batch of events to the HTTP endpoint
func (a *HTTPAdapter) sendBatch(events []*AuctionEvent) error {
	if a.endpoint == "" {
		return nil
	}

	// Create pipe for streaming JSON
	pr, pw := io.Pipe()

	go func() {
		encoder := json.NewEncoder(pw)
		for _, event := range events {
			encoder.Encode(event)
		}
		pw.Close()
	}()

	req, err := http.NewRequest(http.MethodPost, a.endpoint, pr)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-ndjson")
	if a.apiKey != "" {
		req.Header.Set("X-API-Key", a.apiKey)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		logger.Log.Debug().Err(err).Msg("Failed to send analytics batch")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logger.Log.Debug().
			Int("status", resp.StatusCode).
			Int("events", len(events)).
			Msg("Analytics endpoint returned error")
	}

	return nil
}

// MemoryAdapter stores events in memory (for testing)
type MemoryAdapter struct {
	mu     sync.Mutex
	events []*AuctionEvent
	maxSize int
}

// NewMemoryAdapter creates an in-memory analytics adapter
func NewMemoryAdapter(maxSize int) *MemoryAdapter {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &MemoryAdapter{
		events:  make([]*AuctionEvent, 0, maxSize),
		maxSize: maxSize,
	}
}

// Name returns the adapter name
func (a *MemoryAdapter) Name() string {
	return "memory"
}

// LogAuctionEvent stores the event in memory
func (a *MemoryAdapter) LogAuctionEvent(ctx context.Context, event *AuctionEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Evict oldest if at capacity
	if len(a.events) >= a.maxSize {
		a.events = a.events[1:]
	}

	a.events = append(a.events, event)
	return nil
}

// Close is a no-op for memory adapter
func (a *MemoryAdapter) Close() error {
	return nil
}

// GetEvents returns all stored events
func (a *MemoryAdapter) GetEvents() []*AuctionEvent {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make([]*AuctionEvent, len(a.events))
	copy(result, a.events)
	return result
}

// GetEventsByType returns events of a specific type
func (a *MemoryAdapter) GetEventsByType(eventType EventType) []*AuctionEvent {
	a.mu.Lock()
	defer a.mu.Unlock()

	var result []*AuctionEvent
	for _, event := range a.events {
		if event.Type == eventType {
			result = append(result, event)
		}
	}
	return result
}

// Clear removes all stored events
func (a *MemoryAdapter) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = make([]*AuctionEvent, 0, a.maxSize)
}

// Count returns the number of stored events
func (a *MemoryAdapter) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.events)
}
