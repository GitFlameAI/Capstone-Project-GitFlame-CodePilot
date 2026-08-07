package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/observability"
)

type statusResponseWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	w.status = status
	w.written = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// requestLogger emits one structured line per HTTP request. The path is logged
// raw because it carries the repository and issue identifiers needed to debug a
// concrete report; it is deliberately never used as a metric label.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(response, r)
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}
		observability.LoggerFromContext(r.Context()).Log(
			r.Context(), level, "http_request",
			slog.String("event", "http_request"),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Int64("duration_ms", time.Since(started).Milliseconds()),
		)
	})
}

// recoverMiddleware turns a panic into a logged 500 instead of a silently
// dropped connection. net/http already keeps the process alive, but without
// this the stack trace is lost and the client sees an empty response it cannot
// correlate to anything.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response, ok := w.(*statusResponseWriter)
		if !ok {
			response = &statusResponseWriter{ResponseWriter: w}
		}
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			observability.LoggerFromContext(r.Context()).Error(
				"http_panic",
				slog.String("event", "http_panic"),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Any("panic", recovered),
				slog.String("stack", string(debug.Stack())),
			)
			if response.written {
				return
			}
			problem(response, http.StatusInternalServerError, "internal_error", "the request could not be completed")
		}()
		next.ServeHTTP(response, r)
	})
}

// route registers a handler together with the pattern it was registered under.
//
// The pattern is the metric label: `/ai/issues/{id}/plan`, never the concrete
// path with the issue id in it. Go 1.22's ServeMux does expose the matched
// pattern, but only on the request it passes to the handler, which an outer
// middleware never sees — hence binding the label here, at registration time.
func route(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	template := pattern
	if index := strings.IndexByte(pattern, ' '); index >= 0 {
		template = pattern[index+1:]
	}
	mux.Handle(pattern, instrumentRoute(template, handler))
}

func instrumentRoute(template string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response, ok := w.(*statusResponseWriter)
		if !ok {
			response = &statusResponseWriter{ResponseWriter: w}
		}
		next.ServeHTTP(response, r)
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		observability.HTTPRequests.Inc(r.Method, template, strconv.Itoa(status))
		observability.HTTPRequestDuration.Observe(time.Since(started).Seconds(), r.Method, template)
	})
}
