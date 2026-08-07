package httpapi

import (
	"context"
	"log/slog"
	"time"

	"gitflame-codepilot/backend/internal/observability"
	"gitflame-codepilot/backend/internal/queue"
)

// metricsCollectorInterval is how often the gauges are refreshed. Gauges are
// never computed during a scrape: a slow database would then turn into a slow
// /metrics endpoint, and a monitoring system must not be able to hurt the
// service it is watching.
const metricsCollectorInterval = 30 * time.Second

// metricsCollectorTimeout bounds one refresh cycle.
const metricsCollectorTimeout = 5 * time.Second

// StartMetricsCollector publishes the gauges that describe current state:
// dependency health, queue depth, connection token status, and recent task
// counts. It returns immediately and runs until the context is cancelled.
//
// A failing refresh never takes the process down; it increments
// codepilot_collector_errors_total and leaves the previous values in place, so
// a dashboard shows stale numbers rather than a hole.
func (s *Server) StartMetricsCollector(ctx context.Context) {
	observability.PublishBuildInfo("backend")
	go func() {
		ticker := time.NewTicker(metricsCollectorInterval)
		defer ticker.Stop()
		s.collectMetrics(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.collectMetrics(ctx)
			}
		}
	}()
}

func (s *Server) collectMetrics(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, metricsCollectorTimeout)
	defer cancel()

	s.collectDependencyHealth(ctx)
	s.collectQueueDepth(ctx)
	s.collectStoreCounts(ctx)
	s.collectIndexJobs()
}

func (s *Server) collectDependencyHealth(ctx context.Context) {
	for component, check := range s.checks {
		value := 1.0
		if err := check(ctx); err != nil {
			value = 0
			observability.LoggerFromContext(ctx).Warn("dependency_down",
				slog.String("event", "dependency_down"),
				slog.String("component", component),
				slog.String("error", err.Error()))
		}
		observability.DependencyUp.Set(value, component)
	}
}

func (s *Server) collectQueueDepth(ctx context.Context) {
	broker, ok := s.broker.(*queue.RedisBroker)
	if !ok || broker == nil {
		return
	}
	depths, err := broker.Depths(ctx)
	if err != nil {
		observability.CollectorErrors.Inc("queue")
		return
	}
	observability.QueueDepth.Set(float64(depths.Stream), "tasks")
	observability.QueueDepth.Set(float64(depths.DeadLetter), "dead_letter")
	observability.QueuePending.Set(float64(depths.Pending), "tasks")
}

func (s *Server) collectStoreCounts(ctx context.Context) {
	counts, err := s.store.ObservabilityCounts(ctx)
	if err != nil {
		observability.CollectorErrors.Inc("store")
		return
	}
	// Reset before republishing: a status that no longer has any rows must drop
	// off the exposition instead of freezing at its last value.
	observability.GitFlameConnections.Reset()
	for status, total := range counts.ConnectionsByTokenStatus {
		observability.GitFlameConnections.Set(float64(total), status)
	}
	observability.AgentTasksByStatus.Reset()
	for status, total := range counts.TasksByStatusLast24h {
		observability.AgentTasksByStatus.Set(float64(total), status)
	}
}

func (s *Server) collectIndexJobs() {
	observability.RepositoryIndexJobsRunning.Set(float64(s.indexJobs.runningCount()))
}
