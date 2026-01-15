// Package debug provides request tracing and debugging capabilities
package debug

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

// TraceLevel controls the verbosity of debug output
type TraceLevel int

const (
	TraceLevelNone    TraceLevel = 0
	TraceLevelBasic   TraceLevel = 1
	TraceLevelVerbose TraceLevel = 2
	TraceLevelFull    TraceLevel = 3
)

// Trace collects debug information during request processing
type Trace struct {
	mu sync.Mutex

	// Request info
	RequestID string    `json:"request_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`

	// Timing breakdown
	Stages []StageTrace `json:"stages,omitempty"`

	// Bidder traces
	Bidders []BidderTrace `json:"bidders,omitempty"`

	// Warnings and errors
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`

	// Debug messages
	Messages []DebugMessage `json:"messages,omitempty"`

	// Request/Response (if verbose)
	Request  *openrtb.BidRequest  `json:"request,omitempty"`
	Response *openrtb.BidResponse `json:"response,omitempty"`

	// Configuration at time of request
	Config map[string]interface{} `json:"config,omitempty"`
}

// StageTrace tracks timing for a processing stage
type StageTrace struct {
	Name      string        `json:"name"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration_ms"`
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
}

// BidderTrace tracks a single bidder's request/response
type BidderTrace struct {
	BidderCode  string        `json:"bidder_code"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Duration    time.Duration `json:"duration_ms"`
	RequestURL  string        `json:"request_url,omitempty"`
	RequestBody string        `json:"request_body,omitempty"`
	StatusCode  int           `json:"status_code,omitempty"`
	ResponseBody string       `json:"response_body,omitempty"`
	BidCount    int           `json:"bid_count"`
	NoBidReason string        `json:"no_bid_reason,omitempty"`
	Error       string        `json:"error,omitempty"`
}

// DebugMessage is a timestamped debug message
type DebugMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // info, warn, error
	Source    string    `json:"source"`
	Message   string    `json:"message"`
}

// NewTrace creates a new trace for a request
func NewTrace(requestID string) *Trace {
	return &Trace{
		RequestID: requestID,
		StartTime: time.Now(),
		Stages:    make([]StageTrace, 0),
		Bidders:   make([]BidderTrace, 0),
		Warnings:  make([]string, 0),
		Errors:    make([]string, 0),
		Messages:  make([]DebugMessage, 0),
	}
}

// StartStage begins timing a processing stage
func (t *Trace) StartStage(name string) *StageTimer {
	return &StageTimer{
		trace: t,
		stage: StageTrace{
			Name:      name,
			StartTime: time.Now(),
		},
	}
}

// StageTimer tracks a single stage
type StageTimer struct {
	trace *Trace
	stage StageTrace
}

// End completes the stage timing
func (st *StageTimer) End(success bool, err error) {
	st.stage.EndTime = time.Now()
	st.stage.Duration = st.stage.EndTime.Sub(st.stage.StartTime)
	st.stage.Success = success
	if err != nil {
		st.stage.Error = err.Error()
	}

	st.trace.mu.Lock()
	st.trace.Stages = append(st.trace.Stages, st.stage)
	st.trace.mu.Unlock()
}

// StartBidder begins timing a bidder request
func (t *Trace) StartBidder(bidderCode string) *BidderTimer {
	return &BidderTimer{
		trace: t,
		bidder: BidderTrace{
			BidderCode: bidderCode,
			StartTime:  time.Now(),
		},
	}
}

// BidderTimer tracks a single bidder
type BidderTimer struct {
	trace  *Trace
	bidder BidderTrace
}

// SetRequest sets the outgoing request details
func (bt *BidderTimer) SetRequest(url, body string) {
	bt.bidder.RequestURL = url
	bt.bidder.RequestBody = body
}

// SetResponse sets the incoming response details
func (bt *BidderTimer) SetResponse(statusCode int, body string, bidCount int) {
	bt.bidder.StatusCode = statusCode
	bt.bidder.ResponseBody = body
	bt.bidder.BidCount = bidCount
}

// End completes the bidder timing
func (bt *BidderTimer) End(err error) {
	bt.bidder.EndTime = time.Now()
	bt.bidder.Duration = bt.bidder.EndTime.Sub(bt.bidder.StartTime)
	if err != nil {
		bt.bidder.Error = err.Error()
	}

	bt.trace.mu.Lock()
	bt.trace.Bidders = append(bt.trace.Bidders, bt.bidder)
	bt.trace.mu.Unlock()
}

// SetNoBid marks the bidder as returning no bid
func (bt *BidderTimer) SetNoBid(reason string) {
	bt.bidder.NoBidReason = reason
}

// AddWarning adds a warning message
func (t *Trace) AddWarning(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Warnings = append(t.Warnings, msg)
}

// AddError adds an error message
func (t *Trace) AddError(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Errors = append(t.Errors, msg)
}

// AddMessage adds a debug message
func (t *Trace) AddMessage(level, source, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Messages = append(t.Messages, DebugMessage{
		Timestamp: time.Now(),
		Level:     level,
		Source:    source,
		Message:   message,
	})
}

// Info adds an info message
func (t *Trace) Info(source, message string) {
	t.AddMessage("info", source, message)
}

// Warn adds a warning message
func (t *Trace) Warn(source, message string) {
	t.AddMessage("warn", source, message)
	t.AddWarning(message)
}

// Error adds an error message
func (t *Trace) Error(source, message string) {
	t.AddMessage("error", source, message)
	t.AddError(message)
}

// SetRequest stores the original request (for verbose tracing)
func (t *Trace) SetRequest(req *openrtb.BidRequest) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Request = req
}

// SetResponse stores the response (for verbose tracing)
func (t *Trace) SetResponse(resp *openrtb.BidResponse) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Response = resp
}

// SetConfig stores configuration snapshot
func (t *Trace) SetConfig(config map[string]interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Config = config
}

// Complete marks the trace as complete
func (t *Trace) Complete() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.EndTime = time.Now()
}

// Duration returns the total trace duration
func (t *Trace) Duration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	end := t.EndTime
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(t.StartTime)
}

// ToJSON serializes the trace to JSON
func (t *Trace) ToJSON() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return json.Marshal(t)
}

// ToJSONPretty serializes the trace to formatted JSON
func (t *Trace) ToJSONPretty() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return json.MarshalIndent(t, "", "  ")
}

// Summary returns a condensed summary of the trace
func (t *Trace) Summary() *TraceSummary {
	t.mu.Lock()
	defer t.mu.Unlock()

	summary := &TraceSummary{
		RequestID:    t.RequestID,
		Duration:     t.Duration(),
		BidderCount:  len(t.Bidders),
		WarningCount: len(t.Warnings),
		ErrorCount:   len(t.Errors),
		Stages:       make([]string, len(t.Stages)),
	}

	for i, stage := range t.Stages {
		summary.Stages[i] = stage.Name
	}

	totalBids := 0
	for _, bidder := range t.Bidders {
		totalBids += bidder.BidCount
	}
	summary.TotalBids = totalBids

	return summary
}

// TraceSummary is a condensed trace overview
type TraceSummary struct {
	RequestID    string        `json:"request_id"`
	Duration     time.Duration `json:"duration_ms"`
	BidderCount  int           `json:"bidder_count"`
	TotalBids    int           `json:"total_bids"`
	WarningCount int           `json:"warning_count"`
	ErrorCount   int           `json:"error_count"`
	Stages       []string      `json:"stages"`
}

// DebugExtension is added to bid response ext for client debugging
type DebugExtension struct {
	// Trace is the full trace (if requested)
	Trace *Trace `json:"trace,omitempty"`

	// Summary is a condensed overview
	Summary *TraceSummary `json:"summary,omitempty"`

	// Timing breakdown by stage
	Timing map[string]int64 `json:"timing_ms,omitempty"`

	// Bidder summaries
	Bidders []BidderSummary `json:"bidders,omitempty"`
}

// BidderSummary is a condensed bidder trace
type BidderSummary struct {
	Bidder      string `json:"bidder"`
	DurationMS  int64  `json:"duration_ms"`
	BidCount    int    `json:"bid_count"`
	NoBidReason string `json:"no_bid_reason,omitempty"`
	Error       string `json:"error,omitempty"`
}

// BuildDebugExtension creates the debug extension for response
func BuildDebugExtension(trace *Trace, level TraceLevel) *DebugExtension {
	if trace == nil || level == TraceLevelNone {
		return nil
	}

	ext := &DebugExtension{}

	// Basic level - just summary
	if level >= TraceLevelBasic {
		ext.Summary = trace.Summary()

		// Add timing breakdown
		ext.Timing = make(map[string]int64)
		trace.mu.Lock()
		for _, stage := range trace.Stages {
			ext.Timing[stage.Name] = stage.Duration.Milliseconds()
		}
		trace.mu.Unlock()
	}

	// Verbose level - add bidder summaries
	if level >= TraceLevelVerbose {
		trace.mu.Lock()
		ext.Bidders = make([]BidderSummary, len(trace.Bidders))
		for i, b := range trace.Bidders {
			ext.Bidders[i] = BidderSummary{
				Bidder:      b.BidderCode,
				DurationMS:  b.Duration.Milliseconds(),
				BidCount:    b.BidCount,
				NoBidReason: b.NoBidReason,
				Error:       b.Error,
			}
		}
		trace.mu.Unlock()
	}

	// Full level - include full trace
	if level >= TraceLevelFull {
		ext.Trace = trace
	}

	return ext
}
