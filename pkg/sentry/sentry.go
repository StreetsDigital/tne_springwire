// Package sentry provides error tracking integration with Sentry
package sentry

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
)

// Config holds Sentry configuration
type Config struct {
	DSN              string        // Sentry DSN (required)
	Environment      string        // Environment (production, staging, development)
	Release          string        // Release version
	SampleRate       float64       // Error sample rate (0.0 to 1.0)
	TracesSampleRate float64       // Traces sample rate for performance monitoring
	Debug            bool          // Enable debug mode
	FlushTimeout     time.Duration // Timeout for flushing events on shutdown
}

// DefaultConfig returns sensible defaults for Sentry configuration
func DefaultConfig() Config {
	return Config{
		DSN:              os.Getenv("SENTRY_DSN"),
		Environment:      getEnvOrDefault("SENTRY_ENVIRONMENT", "development"),
		Release:          getEnvOrDefault("SENTRY_RELEASE", "1.0.0"),
		SampleRate:       1.0,  // Capture all errors
		TracesSampleRate: 0.1,  // Sample 10% of transactions for performance
		Debug:            os.Getenv("SENTRY_DEBUG") == "true",
		FlushTimeout:     2 * time.Second,
	}
}

// Init initializes the Sentry SDK
// Returns nil if SENTRY_DSN is not set (Sentry disabled)
func Init(cfg Config) error {
	if cfg.DSN == "" {
		return nil // Sentry disabled
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          cfg.Release,
		SampleRate:       cfg.SampleRate,
		TracesSampleRate: cfg.TracesSampleRate,
		Debug:            cfg.Debug,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Add additional context or filter events here
			return event
		},
	})
	if err != nil {
		return fmt.Errorf("sentry initialization failed: %w", err)
	}

	return nil
}

// IsEnabled returns true if Sentry is configured and enabled
func IsEnabled() bool {
	return sentry.CurrentHub().Client() != nil
}

// Flush waits for queued events to be sent
func Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}

// CaptureException captures an error with optional context
func CaptureException(err error) *sentry.EventID {
	if err == nil {
		return nil
	}
	return sentry.CaptureException(err)
}

// CaptureMessage captures a message with optional level
func CaptureMessage(msg string) *sentry.EventID {
	return sentry.CaptureMessage(msg)
}

// WithScope runs a function with a cloned scope for adding context
func WithScope(f func(scope *sentry.Scope)) {
	sentry.WithScope(f)
}

// SetTag sets a tag on the current scope
func SetTag(key, value string) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag(key, value)
	})
}

// SetUser sets user information on the current scope
func SetUser(id, email, ipAddress string) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetUser(sentry.User{
			ID:        id,
			Email:     email,
			IPAddress: ipAddress,
		})
	})
}

// SetContext sets structured context data
func SetContext(key string, value map[string]interface{}) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetContext(key, value)
	})
}

// AddBreadcrumb adds a breadcrumb for debugging
func AddBreadcrumb(category, message string, level sentry.Level) {
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Category:  category,
		Message:   message,
		Level:     level,
		Timestamp: time.Now(),
	})
}

// CaptureError captures an error with additional context
func CaptureError(err error, tags map[string]string, extras map[string]interface{}) *sentry.EventID {
	if err == nil {
		return nil
	}

	var eventID *sentry.EventID
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		for k, v := range extras {
			scope.SetExtra(k, v)
		}
		eventID = sentry.CaptureException(err)
	})
	return eventID
}

// RecoverWithSentry recovers from panics and reports to Sentry
// Use with defer: defer sentry.RecoverWithSentry()
func RecoverWithSentry() {
	if r := recover(); r != nil {
		var err error
		switch v := r.(type) {
		case error:
			err = v
		case string:
			err = fmt.Errorf("%s", v)
		default:
			err = fmt.Errorf("panic: %v", v)
		}

		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelFatal)
			scope.SetExtra("stacktrace", string(debug.Stack()))
			sentry.CaptureException(err)
		})

		// Re-panic after capturing
		panic(r)
	}
}

// RecoverAndLogWithSentry recovers from panics, reports to Sentry, and logs
// Does not re-panic - use for goroutines that should not crash the server
func RecoverAndLogWithSentry(logger func(msg string, err error)) {
	if r := recover(); r != nil {
		var err error
		switch v := r.(type) {
		case error:
			err = v
		case string:
			err = fmt.Errorf("%s", v)
		default:
			err = fmt.Errorf("panic: %v", v)
		}

		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelFatal)
			scope.SetExtra("stacktrace", string(debug.Stack()))
			sentry.CaptureException(err)
		})

		if logger != nil {
			logger("recovered from panic", err)
		}
	}
}

// HTTPMiddleware returns middleware that captures panics and adds request context
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		hub := sentry.GetHubFromContext(ctx)
		if hub == nil {
			hub = sentry.CurrentHub().Clone()
			ctx = sentry.SetHubOnContext(ctx, hub)
		}

		// Add request context
		hub.Scope().SetRequest(r)
		hub.Scope().SetTag("http.method", r.Method)
		hub.Scope().SetTag("http.url", r.URL.Path)

		defer func() {
			if r := recover(); r != nil {
				var err error
				switch v := r.(type) {
				case error:
					err = v
				case string:
					err = fmt.Errorf("%s", v)
				default:
					err = fmt.Errorf("panic: %v", v)
				}

				hub.WithScope(func(scope *sentry.Scope) {
					scope.SetLevel(sentry.LevelFatal)
					scope.SetExtra("stacktrace", string(debug.Stack()))
				})
				hub.CaptureException(err)
				hub.Flush(2 * time.Second)

				// Return 500 error
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// StartSpan starts a new span for performance monitoring
func StartSpan(ctx context.Context, operation string) *sentry.Span {
	return sentry.StartSpan(ctx, operation)
}

// ContextWithSpan returns a context with the given span
func ContextWithSpan(ctx context.Context, span *sentry.Span) context.Context {
	return span.Context()
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
