package observability

import (
	"bytes"
	"net/http"
	"time"
)

// The metrics that describe GitFlame CodePilot itself. Container CPU and memory
// come from the container runtime; what nobody else can report is how the AI
// pipeline is doing, so that is what is measured here.
//
// Label discipline: repository ids, issue ids, task ids and file paths are never
// labels. They are unbounded, and one series per repository would eventually
// take Prometheus down. They belong in the logs, correlated by request id.
var (
	HTTPRequests = NewCounterVec(
		"codepilot_http_requests_total",
		"HTTP requests handled by the service.",
		"method", "route", "status",
	)
	HTTPRequestDuration = NewHistogramVec(
		"codepilot_http_request_duration_seconds",
		"HTTP request duration.",
		[]float64{0.05, 0.1, 0.5, 1, 5, 15, 60, 180, 600},
		"method", "route",
	)

	UpstreamRequests = NewCounterVec(
		"codepilot_upstream_requests_total",
		"Requests to upstream services, by outcome.",
		"upstream", "outcome",
	)
	UpstreamRequestDuration = NewHistogramVec(
		"codepilot_upstream_request_duration_seconds",
		"Upstream request duration.",
		[]float64{0.05, 0.5, 1, 5, 15, 60, 300, 900},
		"upstream",
	)

	AgentTasksExecuted = NewCounterVec(
		"codepilot_agent_tasks_executed_total",
		"Agent tasks executed by this process, by outcome.",
		"task_type", "outcome",
	)
	AgentTaskDuration = NewHistogramVec(
		"codepilot_agent_task_duration_seconds",
		"Agent task execution duration.",
		[]float64{5, 15, 30, 60, 120, 300, 600, 1200},
		"task_type",
	)
	AgentTaskTokens = NewCounterVec(
		"codepilot_agent_task_tokens_total",
		"Model tokens attributed to agent tasks.",
		"task_type", "kind",
	)
	AgentTaskRetries = NewCounterVec(
		"codepilot_agent_task_retries_total",
		"Agent tasks re-queued after a failure.",
		"task_type",
	)
	AgentTasksDeadLettered = NewCounterVec(
		"codepilot_agent_tasks_dead_lettered_total",
		"Agent tasks that exhausted their retries and were moved to the dead-letter stream.",
		"task_type",
	)

	RepositoryIndexJobs = NewCounterVec(
		"codepilot_repository_index_jobs_total",
		"Background repository indexing jobs, by outcome.",
		"outcome",
	)
	RepositoryIndexDuration = NewHistogramVec(
		"codepilot_repository_index_duration_seconds",
		"Background repository indexing duration.",
		[]float64{5, 15, 30, 60, 120, 300, 600, 1200},
	)
	RepositoryIndexJobsRunning = NewGaugeVec(
		"codepilot_repository_index_jobs_running",
		"Repository indexing jobs currently running in this process.",
	)

	QueueDepth = NewGaugeVec(
		"codepilot_queue_depth",
		"Entries in a Redis stream.",
		"stream",
	)
	QueuePending = NewGaugeVec(
		"codepilot_queue_pending",
		"Entries delivered to a consumer but not yet acknowledged.",
		"stream",
	)

	DependencyUp = NewGaugeVec(
		"codepilot_dependency_up",
		"1 when a dependency answered its readiness check, 0 otherwise.",
		"component",
	)

	GitFlameConnections = NewGaugeVec(
		"codepilot_gitflame_connections",
		"Stored GitFlame connections by token status.",
		"token_status",
	)

	AgentTasksByStatus = NewGaugeVec(
		"codepilot_agent_tasks",
		"Agent tasks created in the last 24 hours, by status.",
		"status",
	)

	CollectorErrors = NewCounterVec(
		"codepilot_collector_errors_total",
		"Failures of the background metrics collector.",
		"source",
	)

	BuildInfoMetric = NewGaugeVec(
		"codepilot_build_info",
		"Always 1; the labels carry the build identity of the running process.",
		"service", "version", "commit", "go_version",
	)
)

// PublishBuildInfo records the build identity as a metric so that a dashboard
// can show which version produced a graph, and an alert can catch a rollback.
func PublishBuildInfo(service string) {
	build := Build(service)
	BuildInfoMetric.Set(1, build.Service, build.Version, build.Commit, build.GoVersion)
}

// MetricsHandler serves the exposition. It is never exposed through the public
// nginx route: Prometheus scrapes it over the internal Docker network.
func MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var buffer bytes.Buffer
		DefaultRegistry.Render(&buffer)
		w.Header().Set("Content-Type", ContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buffer.Bytes())
	})
}

// ObserveUpstream records one call to an upstream service. `outcome` is a small
// fixed vocabulary: success, client_error, server_error, unreachable, timeout.
func ObserveUpstream(upstream, outcome string, started time.Time) {
	UpstreamRequests.Inc(upstream, outcome)
	UpstreamRequestDuration.Observe(time.Since(started).Seconds(), upstream)
}

// UpstreamOutcome maps an HTTP status to the outcome vocabulary.
func UpstreamOutcome(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return "success"
	}
}
