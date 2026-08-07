// Package observability holds the cross-cutting concerns that make the running
// system explainable: structured logging, request correlation, and build
// identity. It deliberately depends on nothing but the standard library, so it
// can be imported from every binary without pulling the domain along.
package observability

import (
	"log/slog"
	"os"
	"strings"
)

// Init installs the process-wide logger and returns it.
//
// Every service in GitFlame CodePilot writes one JSON object per line to stdout
// with the same base fields, so a single query can follow one request across
// the backend, the worker, and the Python services:
//
//	{"time":...,"level":"INFO","msg":"...","service":"backend","request_id":"..."}
//
// LOG_FORMAT=text switches to the human-readable handler for local debugging.
// LOG_LEVEL accepts debug, info, warn, and error; anything else falls back to
// info rather than failing to start, because a typo in an environment variable
// must never take the service down.
func Init(service, level, format string) *slog.Logger {
	options := &slog.HandlerOptions{Level: parseLevel(level)}
	var handler slog.Handler
	if strings.EqualFold(strings.TrimSpace(format), "text") {
		handler = slog.NewTextHandler(os.Stdout, options)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, options)
	}
	logger := slog.New(handler).With(slog.String("service", service))
	slog.SetDefault(logger)
	return logger
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
