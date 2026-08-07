package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitflame-codepilot/backend/internal/observability"
)

// captureLogs redirects the default logger into a buffer for the duration of a
// test and returns the decoded records.
func captureLogs(t *testing.T, run func()) []map[string]any {
	t.Helper()
	previous := slog.Default()
	defer slog.SetDefault(previous)

	var buffer strings.Builder
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug})))
	run()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not valid JSON: %q (%v)", line, err)
		}
		records = append(records, record)
	}
	return records
}

func TestRequestLoggerEmitsStructuredLineWithRequestID(t *testing.T) {
	handler := observability.RequestIDMiddleware(requestLogger(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// The handler must see the same identifier that the client sent.
			if got := observability.RequestIDFromContext(r.Context()); got != "trace-1" {
				t.Errorf("request id in context = %q, want %q", got, "trace-1")
			}
			w.WriteHeader(http.StatusCreated)
		})))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ai/issues/42/approve", nil)
	request.Header.Set(observability.RequestIDHeader, "trace-1")

	records := captureLogs(t, func() { handler.ServeHTTP(recorder, request) })

	if got := recorder.Header().Get(observability.RequestIDHeader); got != "trace-1" {
		t.Fatalf("echoed request id = %q, want %q", got, "trace-1")
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly one log record, got %d", len(records))
	}
	record := records[0]
	if record["event"] != "http_request" || record["request_id"] != "trace-1" {
		t.Fatalf("record = %#v", record)
	}
	if record["status"] != float64(http.StatusCreated) || record["path"] != "/ai/issues/42/approve" {
		t.Fatalf("record = %#v", record)
	}
	if _, ok := record["duration_ms"]; !ok {
		t.Fatalf("record is missing duration_ms: %#v", record)
	}
}

func TestRequestLoggerGeneratesRequestIDAndRaisesLevelOnServerError(t *testing.T) {
	handler := observability.RequestIDMiddleware(requestLogger(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})))

	recorder := httptest.NewRecorder()
	records := captureLogs(t, func() {
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	})

	generated := recorder.Header().Get(observability.RequestIDHeader)
	if len(generated) != 32 {
		t.Fatalf("generated request id = %q, want 32 hex characters", generated)
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly one log record, got %d", len(records))
	}
	if records[0]["level"] != "ERROR" || records[0]["request_id"] != generated {
		t.Fatalf("record = %#v", records[0])
	}
}

func TestRecoverMiddlewareTurnsPanicIntoLoggedProblemResponse(t *testing.T) {
	handler := observability.RequestIDMiddleware(requestLogger(recoverMiddleware(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}))))

	recorder := httptest.NewRecorder()
	records := captureLogs(t, func() {
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ai/tasks/1", nil))
	})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON: %q", recorder.Body.String())
	}
	if body["code"] != "internal_error" {
		t.Fatalf("body = %#v", body)
	}
	// The panic must be reported, and the request must still be logged as a
	// normal 500 so it is not invisible to whoever watches the logs.
	var sawPanic, sawRequest bool
	for _, record := range records {
		switch record["event"] {
		case "http_panic":
			sawPanic = true
			if stack, _ := record["stack"].(string); !strings.Contains(stack, "panic") {
				t.Errorf("http_panic record has no usable stack: %#v", record)
			}
		case "http_request":
			sawRequest = true
			if record["status"] != float64(http.StatusInternalServerError) {
				t.Errorf("http_request status = %v, want 500", record["status"])
			}
		}
	}
	if !sawPanic || !sawRequest {
		t.Fatalf("records = %#v", records)
	}
}

func TestRequestIDMiddlewareRejectsUnsafeInboundValues(t *testing.T) {
	var seen string
	handler := observability.RequestIDMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = observability.RequestIDFromContext(r.Context())
	}))

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set(observability.RequestIDHeader, "abc\ninjected line\"quote")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if seen != "abcinjectedlinequote" {
		t.Fatalf("sanitized request id = %q", seen)
	}
}
