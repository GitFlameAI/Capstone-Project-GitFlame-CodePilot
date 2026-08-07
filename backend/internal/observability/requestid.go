package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
)

// RequestIDHeader is the header used to correlate one user action across the
// backend, the agent worker, the Agent Engine, the recommendation service, and
// CodeRAG. An incoming value is reused so that a caller (or GitFlame itself)
// can trace a request end to end; otherwise a new one is generated.
const RequestIDHeader = "X-Request-ID"

// requestIDMaxLength bounds a caller-supplied identifier. Log lines and
// downstream headers must not be steerable by an arbitrarily long input.
const requestIDMaxLength = 64

type contextKey struct{}

// NewRequestID returns a random 128-bit identifier in hex.
func NewRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		// crypto/rand only fails if the system entropy source is broken. The
		// identifier is for correlation, not for security, so an empty value is
		// worse than a degraded one: fall back to a fixed marker that is still
		// obviously distinguishable in the logs.
		return "no-request-id"
	}
	return hex.EncodeToString(buffer)
}

// WithRequestID stores a request identifier in the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if strings.TrimSpace(requestID) == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, requestID)
}

// RequestIDFromContext returns the request identifier, or an empty string for
// work that did not originate from an HTTP request.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(contextKey{}).(string)
	return value
}

// LoggerFromContext returns the default logger annotated with the request
// identifier when there is one, so handlers can log without threading a logger
// through every signature.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	requestID := RequestIDFromContext(ctx)
	if requestID == "" {
		return slog.Default()
	}
	return slog.Default().With(slog.String("request_id", requestID))
}

// PropagateRequestID copies the identifier of the current context onto an
// outgoing request. Every HTTP client in the backend calls it, which is what
// makes a single grep across all services return the full story of one action.
func PropagateRequestID(ctx context.Context, request *http.Request) {
	if request == nil {
		return
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		request.Header.Set(RequestIDHeader, requestID)
	}
}

// RequestIDMiddleware accepts or generates a request identifier, puts it in the
// request context, and echoes it back so a user can quote it in a bug report.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := sanitizeRequestID(r.Header.Get(RequestIDHeader))
		if requestID == "" {
			requestID = NewRequestID()
		}
		w.Header().Set(RequestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), requestID)))
	})
}

// sanitizeRequestID keeps only characters that are safe to echo into logs and
// into downstream headers, and drops the value entirely if nothing is left.
func sanitizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > requestIDMaxLength {
		value = value[:requestIDMaxLength]
	}
	var builder strings.Builder
	for _, symbol := range value {
		switch {
		case symbol >= 'a' && symbol <= 'z',
			symbol >= 'A' && symbol <= 'Z',
			symbol >= '0' && symbol <= '9',
			symbol == '-', symbol == '_':
			builder.WriteRune(symbol)
		}
	}
	return builder.String()
}
