package debug

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
)

func TestTrace_Basic(t *testing.T) {
	trace := NewTrace("test-123")

	if trace.RequestID != "test-123" {
		t.Errorf("expected request ID 'test-123', got '%s'", trace.RequestID)
	}

	if trace.StartTime.IsZero() {
		t.Error("expected start time to be set")
	}
}

func TestTrace_Stages(t *testing.T) {
	trace := NewTrace("test-123")

	// Start and end a stage
	stage := trace.StartStage("validation")
	time.Sleep(10 * time.Millisecond)
	stage.End(true, nil)

	if len(trace.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(trace.Stages))
	}

	if trace.Stages[0].Name != "validation" {
		t.Errorf("expected stage name 'validation', got '%s'", trace.Stages[0].Name)
	}

	if !trace.Stages[0].Success {
		t.Error("expected stage to be successful")
	}

	if trace.Stages[0].Duration < 10*time.Millisecond {
		t.Error("expected duration >= 10ms")
	}
}

func TestTrace_StageWithError(t *testing.T) {
	trace := NewTrace("test-123")

	stage := trace.StartStage("processing")
	stage.End(false, errors.New("something went wrong"))

	if trace.Stages[0].Success {
		t.Error("expected stage to be unsuccessful")
	}

	if trace.Stages[0].Error != "something went wrong" {
		t.Errorf("expected error message, got '%s'", trace.Stages[0].Error)
	}
}

func TestTrace_Bidders(t *testing.T) {
	trace := NewTrace("test-123")

	bidder := trace.StartBidder("appnexus")
	bidder.SetRequest("https://api.appnexus.com", `{"id":"1"}`)
	time.Sleep(5 * time.Millisecond)
	bidder.SetResponse(200, `{"bids":[]}`, 2)
	bidder.End(nil)

	if len(trace.Bidders) != 1 {
		t.Fatalf("expected 1 bidder, got %d", len(trace.Bidders))
	}

	b := trace.Bidders[0]
	if b.BidderCode != "appnexus" {
		t.Errorf("expected bidder 'appnexus', got '%s'", b.BidderCode)
	}

	if b.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", b.StatusCode)
	}

	if b.BidCount != 2 {
		t.Errorf("expected 2 bids, got %d", b.BidCount)
	}
}

func TestTrace_NoBid(t *testing.T) {
	trace := NewTrace("test-123")

	bidder := trace.StartBidder("rubicon")
	bidder.SetNoBid("no inventory")
	bidder.End(nil)

	if trace.Bidders[0].NoBidReason != "no inventory" {
		t.Errorf("expected no bid reason, got '%s'", trace.Bidders[0].NoBidReason)
	}
}

func TestTrace_Messages(t *testing.T) {
	trace := NewTrace("test-123")

	trace.Info("auction", "Starting auction")
	trace.Warn("privacy", "GDPR consent missing")
	trace.Error("bidder", "Timeout occurred")

	if len(trace.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(trace.Messages))
	}

	if len(trace.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(trace.Warnings))
	}

	if len(trace.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(trace.Errors))
	}
}

func TestTrace_SetRequest(t *testing.T) {
	trace := NewTrace("test-123")

	req := &openrtb.BidRequest{ID: "req-1"}
	trace.SetRequest(req)

	if trace.Request == nil || trace.Request.ID != "req-1" {
		t.Error("expected request to be set")
	}
}

func TestTrace_SetResponse(t *testing.T) {
	trace := NewTrace("test-123")

	resp := &openrtb.BidResponse{ID: "resp-1"}
	trace.SetResponse(resp)

	if trace.Response == nil || trace.Response.ID != "resp-1" {
		t.Error("expected response to be set")
	}
}

func TestTrace_Complete(t *testing.T) {
	trace := NewTrace("test-123")

	if !trace.EndTime.IsZero() {
		t.Error("end time should be zero before complete")
	}

	trace.Complete()

	if trace.EndTime.IsZero() {
		t.Error("end time should be set after complete")
	}
}

func TestTrace_Duration(t *testing.T) {
	trace := NewTrace("test-123")

	time.Sleep(10 * time.Millisecond)
	trace.Complete()

	duration := trace.Duration()
	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}
}

func TestTrace_Summary(t *testing.T) {
	trace := NewTrace("test-123")

	// Add some data
	stage := trace.StartStage("validation")
	stage.End(true, nil)

	bidder := trace.StartBidder("appnexus")
	bidder.SetResponse(200, "", 3)
	bidder.End(nil)

	trace.AddWarning("test warning")
	trace.Complete()

	summary := trace.Summary()

	if summary.RequestID != "test-123" {
		t.Errorf("expected request ID 'test-123', got '%s'", summary.RequestID)
	}

	if summary.BidderCount != 1 {
		t.Errorf("expected 1 bidder, got %d", summary.BidderCount)
	}

	if summary.TotalBids != 3 {
		t.Errorf("expected 3 total bids, got %d", summary.TotalBids)
	}

	if summary.WarningCount != 1 {
		t.Errorf("expected 1 warning, got %d", summary.WarningCount)
	}

	if len(summary.Stages) != 1 {
		t.Errorf("expected 1 stage, got %d", len(summary.Stages))
	}
}

func TestTrace_ToJSON(t *testing.T) {
	trace := NewTrace("test-123")
	trace.Info("test", "test message")
	trace.Complete()

	jsonBytes, err := trace.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	var decoded Trace
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.RequestID != "test-123" {
		t.Errorf("expected request ID 'test-123', got '%s'", decoded.RequestID)
	}
}

func TestTrace_ToJSONPretty(t *testing.T) {
	trace := NewTrace("test-123")
	trace.Complete()

	jsonBytes, err := trace.ToJSONPretty()
	if err != nil {
		t.Fatal(err)
	}

	// Pretty JSON should have newlines
	hasNewline := false
	for _, b := range jsonBytes {
		if b == '\n' {
			hasNewline = true
			break
		}
	}
	if !hasNewline {
		t.Error("expected pretty JSON to have newlines")
	}
}

func TestBuildDebugExtension_None(t *testing.T) {
	trace := NewTrace("test-123")
	ext := BuildDebugExtension(trace, TraceLevelNone)

	if ext != nil {
		t.Error("expected nil extension for TraceLevelNone")
	}
}

func TestBuildDebugExtension_Basic(t *testing.T) {
	trace := NewTrace("test-123")
	stage := trace.StartStage("test")
	stage.End(true, nil)
	trace.Complete()

	ext := BuildDebugExtension(trace, TraceLevelBasic)

	if ext == nil {
		t.Fatal("expected extension")
	}

	if ext.Summary == nil {
		t.Error("expected summary at basic level")
	}

	if ext.Timing == nil {
		t.Error("expected timing at basic level")
	}

	if ext.Bidders != nil {
		t.Error("expected no bidders at basic level")
	}
}

func TestBuildDebugExtension_Verbose(t *testing.T) {
	trace := NewTrace("test-123")
	bidder := trace.StartBidder("test")
	bidder.SetResponse(200, "", 1)
	bidder.End(nil)
	trace.Complete()

	ext := BuildDebugExtension(trace, TraceLevelVerbose)

	if ext == nil {
		t.Fatal("expected extension")
	}

	if ext.Bidders == nil || len(ext.Bidders) != 1 {
		t.Error("expected bidders at verbose level")
	}

	if ext.Trace != nil {
		t.Error("expected no full trace at verbose level")
	}
}

func TestBuildDebugExtension_Full(t *testing.T) {
	trace := NewTrace("test-123")
	trace.Complete()

	ext := BuildDebugExtension(trace, TraceLevelFull)

	if ext == nil {
		t.Fatal("expected extension")
	}

	if ext.Trace == nil {
		t.Error("expected full trace at full level")
	}
}

func TestBuildDebugExtension_NilTrace(t *testing.T) {
	ext := BuildDebugExtension(nil, TraceLevelFull)

	if ext != nil {
		t.Error("expected nil extension for nil trace")
	}
}
