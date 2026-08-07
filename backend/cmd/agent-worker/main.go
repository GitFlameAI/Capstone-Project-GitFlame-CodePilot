package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitflame-codepilot/backend/internal/agent"
	"gitflame-codepilot/backend/internal/config"
	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/observability"
	"gitflame-codepilot/backend/internal/queue"
	"gitflame-codepilot/backend/internal/repository"
	"gitflame-codepilot/backend/internal/service"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check worker dependencies and exit")
	flag.Parse()
	cfg := config.Load()
	logger := observability.Init("agent-worker", cfg.LogLevel, cfg.LogFormat)
	if cfg.DatabaseURL == "" || cfg.RedisURL == "" {
		fatal(logger, "configuration_invalid", errors.New("agent-worker requires DATABASE_URL and REDIS_URL"))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := repository.NewPostgresStore(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal(logger, "storage_unavailable", err)
	}
	defer store.Close()
	broker, err := queue.NewRedisBroker(cfg.RedisURL, cfg.AgentQueueName, cfg.AgentConsumerGroup, cfg.QueueMaxLength)
	if err != nil {
		fatal(logger, "queue_unavailable", err)
	}
	engine := agent.NewClient(cfg.AgentEngineURL, cfg.AgentTimeout)
	if *healthcheck {
		if err := broker.Ping(ctx); err != nil {
			fatal(logger, "healthcheck_failed", err)
		}
		if err := engine.Ready(ctx); err != nil {
			fatal(logger, "healthcheck_failed", err)
		}
		return
	}
	if err := broker.EnsureGroup(ctx); err != nil {
		fatal(logger, "queue_group_failed", err)
	}
	workflow := service.NewWorkflow(store, engine)
	hostname, _ := os.Hostname()
	consumer := hostname + "-1"
	startMetricsServer(logger, cfg.WorkerMetricsAddr, broker)
	build := observability.Build("agent-worker")
	logger.Info("worker_started",
		slog.String("event", "worker_started"),
		slog.String("stream", cfg.AgentQueueName),
		slog.String("consumer", consumer),
		slog.String("version", build.Version),
		slog.String("commit", build.Commit),
		slog.Int("concurrency", 1),
	)
	for ctx.Err() == nil {
		message, err := broker.Read(ctx, consumer)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if message != nil && message.ID != "" {
				if deadLetterErr := broker.DeadLetter(ctx, *message, err); deadLetterErr == nil {
					_ = broker.Ack(ctx, message.ID)
				}
			}
			logger.Error("task_read_failed",
				slog.String("event", "task_read_failed"), slog.String("error", err.Error()))
			continue
		}
		if message == nil {
			continue
		}
		// A task that is already running is detached from the shutdown signal:
		// interrupting a model call in the middle wastes the tokens already
		// spent and leaves the task to be retried from scratch. New tasks are
		// not picked up, because the loop condition re-checks ctx.
		taskCtx, cancelTask := context.WithTimeout(
			context.WithoutCancel(ctx), cfg.AgentTimeout+shutdownGraceMargin)
		process(taskCtx, logger, workflow, broker, *message, cfg.WorkerMaxRetries)
		cancelTask()
	}
	logger.Info("shutdown_complete", slog.String("event", "shutdown_complete"))
}

// shutdownGraceMargin is added to the agent timeout so a task that is finishing
// during shutdown has room to write its result to the database.
const shutdownGraceMargin = 30 * time.Second

type taskExecutor interface {
	ExecuteTask(context.Context, domain.AgentJob) error
	RetryTask(domain.AgentJob) error
}

func process(ctx context.Context, logger *slog.Logger, workflow taskExecutor, broker queue.Broker, message queue.Message, maxRetries int) {
	// The Agent Engine correlates work by the request id carried in the job body,
	// which the workflow sets to the task id. Reusing it as the HTTP correlation
	// id means the worker, the engine, and the task row in Postgres all speak
	// about the same identifier.
	requestID := jobRequestID(message.Job)
	ctx = observability.WithRequestID(ctx, requestID)
	logger = logger.With(
		slog.String("task_id", message.Job.TaskID),
		slog.String("task_type", message.Job.Type),
		slog.String("request_id", requestID),
	)
	err := workflow.ExecuteTask(ctx, message.Job)
	if err == nil {
		if ackErr := broker.Ack(ctx, message.ID); ackErr != nil {
			logger.Error("task_ack_failed",
				slog.String("event", "task_ack_failed"), slog.String("error", ackErr.Error()))
		}
		return
	}
	if temporary(err) && message.Job.Attempt < maxRetries {
		message.Job.Attempt++
		if retryErr := workflow.RetryTask(message.Job); retryErr == nil {
			if publishErr := broker.Publish(ctx, message.Job); publishErr == nil {
				_ = broker.Ack(ctx, message.ID)
				observability.AgentTaskRetries.Inc(message.Job.Type)
				logger.Warn("task_retry_scheduled",
					slog.String("event", "task_retry_scheduled"),
					slog.Int("attempt", message.Job.Attempt), slog.Int("max_retries", maxRetries),
					slog.String("error", err.Error()))
				return
			}
		}
	}
	if deadLetterErr := broker.DeadLetter(ctx, message, err); deadLetterErr != nil {
		logger.Error("task_dead_letter_failed",
			slog.String("event", "task_dead_letter_failed"),
			slog.String("error", deadLetterErr.Error()), slog.String("cause", err.Error()))
		return
	}
	_ = broker.Ack(ctx, message.ID)
	observability.AgentTasksDeadLettered.Inc(message.Job.Type)
	logger.Error("task_failed_permanently",
		slog.String("event", "task_failed_permanently"), slog.String("error", err.Error()))
}

func temporary(err error) bool {
	var engineError *agent.Error
	if !errors.As(err, &engineError) {
		return false
	}
	return (engineError.Status == http.StatusBadGateway &&
		(engineError.Code == "agent_engine_unreachable" || engineError.Code == "agent_engine_error")) ||
		engineError.Status == http.StatusServiceUnavailable ||
		engineError.Status == http.StatusGatewayTimeout
}

// startMetricsServer exposes the worker over HTTP. The worker is otherwise a
// pure queue consumer with no port, which made it the one component nobody
// could see into: no metrics, no version, no readiness beyond the compose
// healthcheck flag.
func startMetricsServer(logger *slog.Logger, addr string, broker *queue.RedisBroker) {
	observability.PublishBuildInfo("agent-worker")
	go collectQueueDepth(broker)

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", observability.MetricsHandler())
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"agent-worker"}`))
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(observability.Build("agent-worker"))
	})

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// The worker must keep consuming tasks even if its metrics port is
			// taken, so this is logged and not fatal.
			logger.Error("metrics_server_failed",
				slog.String("event", "metrics_server_failed"),
				slog.String("addr", addr), slog.String("error", err.Error()))
		}
	}()
	logger.Info("metrics_server_started",
		slog.String("event", "metrics_server_started"), slog.String("addr", addr))
}

// collectQueueDepth republishes the queue gauges from the worker as well as the
// backend, so the numbers stay visible even when the backend is down.
func collectQueueDepth(broker *queue.RedisBroker) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		depths, err := broker.Depths(ctx)
		cancel()
		if err != nil {
			observability.CollectorErrors.Inc("queue")
		} else {
			observability.QueueDepth.Set(float64(depths.Stream), "tasks")
			observability.QueueDepth.Set(float64(depths.DeadLetter), "dead_letter")
			observability.QueuePending.Set(float64(depths.Pending), "tasks")
		}
		<-ticker.C
	}
}

func jobRequestID(job domain.AgentJob) string {
	if job.Request.RequestID != "" {
		return job.Request.RequestID
	}
	return job.CodeGenerationRequest.RequestID
}

// fatal reports a startup failure in the same structured format as every other
// log line and stops the process. log.Fatal would bypass the JSON handler and
// produce a line that no log query can find.
func fatal(logger *slog.Logger, event string, err error) {
	logger.Error(event, slog.String("event", event), slog.String("error", err.Error()))
	os.Exit(1)
}
