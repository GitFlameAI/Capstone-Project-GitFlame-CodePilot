package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"gitflame-codepilot/backend/internal/observability"
	"gitflame-codepilot/backend/internal/queue"
	"gitflame-codepilot/backend/internal/repository"
)

// The /ops endpoints answer "is this deployment healthy, and if not, what
// broke" without requiring Prometheus, Grafana or shell access to the VM. They
// exist because after the handover the first person to look at a problem may
// have nothing but a browser and this service.
//
// Three rules hold for everything in this file:
//   - read-only: no endpoint here changes state;
//   - authenticated: an application session is required, because the payload
//     describes the internals of the deployment;
//   - no content: identifiers, statuses, counts and error codes only. Prompts,
//     generated code, repository files and tokens never appear.

const (
	opsStatusTimeout   = 3 * time.Second
	opsDefaultTaskList = 50
	opsMaxTaskList     = 200
	opsDeadLetterLimit = 20
)

type opsDependency struct {
	Component string `json:"component"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type opsStatusResponse struct {
	Service      string                  `json:"service"`
	Build        observability.BuildInfo `json:"build"`
	Status       string                  `json:"status"`
	Dependencies []opsDependency         `json:"dependencies"`
	Queue        *opsQueue               `json:"queue,omitempty"`
	Tasks24h     map[string]int          `json:"tasks_last_24h"`
	Connections  map[string]int          `json:"connections_by_token_status"`
	Indexing     opsIndexing             `json:"indexing"`
	GeneratedAt  time.Time               `json:"generated_at"`
	Warnings     []string                `json:"warnings,omitempty"`
}

type opsQueue struct {
	Stream     int64 `json:"stream"`
	Pending    int64 `json:"pending"`
	DeadLetter int64 `json:"dead_letter"`
}

type opsIndexing struct {
	Running int `json:"running"`
}

// opsStatus is the single screen an operator opens first.
func (s *Server) opsStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authenticate(r); err != nil {
		authError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), opsStatusTimeout)
	defer cancel()

	response := opsStatusResponse{
		Service:     "backend",
		Build:       observability.Build("backend"),
		Status:      "ok",
		Tasks24h:    map[string]int{},
		Connections: map[string]int{},
		Indexing:    opsIndexing{Running: s.indexJobs.runningCount()},
		GeneratedAt: time.Now().UTC(),
	}

	for component, check := range s.checks {
		dependency := opsDependency{Component: component, Status: "ok"}
		if err := check(ctx); err != nil {
			dependency.Status = "down"
			dependency.Error = truncateDetail(err.Error(), indexJobErrorLimit)
			response.Status = "degraded"
		}
		response.Dependencies = append(response.Dependencies, dependency)
	}
	sortDependencies(response.Dependencies)

	if broker, ok := s.broker.(*queue.RedisBroker); ok && broker != nil {
		if depths, err := broker.Depths(ctx); err == nil {
			response.Queue = &opsQueue{Stream: depths.Stream, Pending: depths.Pending, DeadLetter: depths.DeadLetter}
		} else {
			// A partial answer is more useful than a 500: the rest of the page
			// still tells the operator what is working.
			response.Warnings = append(response.Warnings, "queue depth is unavailable")
		}
	}

	if counts, err := s.store.ObservabilityCounts(ctx); err == nil {
		response.Tasks24h = counts.TasksByStatusLast24h
		response.Connections = counts.ConnectionsByTokenStatus
	} else {
		response.Warnings = append(response.Warnings, "task and connection counts are unavailable")
	}

	write(w, http.StatusOK, response)
}

// opsTasks lists the most recent agent tasks, newest first.
func (s *Server) opsTasks(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authenticate(r); err != nil {
		authError(w, err)
		return
	}
	query := repository.RecentTasksQuery{
		Limit:  opsDefaultTaskList,
		Status: r.URL.Query().Get("status"),
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > opsMaxTaskList {
			problem(w, http.StatusBadRequest, "invalid_limit",
				"limit must be a positive integer not greater than "+strconv.Itoa(opsMaxTaskList))
			return
		}
		query.Limit = parsed
	}
	tasks, err := s.store.RecentAgentTasks(r.Context(), query)
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"tasks": tasks, "count": len(tasks)})
}

// opsDeadLetter lists tasks that exhausted their retries. Without it, a
// dead-lettered task is invisible: it is not in the database as pending, and
// nobody is watching the Redis stream by hand.
func (s *Server) opsDeadLetter(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authenticate(r); err != nil {
		authError(w, err)
		return
	}
	broker, ok := s.broker.(*queue.RedisBroker)
	if !ok || broker == nil {
		write(w, http.StatusOK, map[string]any{
			"entries": []queue.DeadLetterEntry{}, "count": 0,
			"detail": "the dead-letter stream exists only when TASK_DISPATCH_MODE=redis",
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), opsStatusTimeout)
	defer cancel()
	entries, err := broker.DeadLetterEntries(ctx, opsDeadLetterLimit)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "queue_unavailable", err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"entries": entries, "count": len(entries)})
}

func sortDependencies(dependencies []opsDependency) {
	// Deterministic order: map iteration would otherwise reshuffle the page on
	// every refresh, which makes it much harder to read.
	for i := 1; i < len(dependencies); i++ {
		for j := i; j > 0 && dependencies[j].Component < dependencies[j-1].Component; j-- {
			dependencies[j], dependencies[j-1] = dependencies[j-1], dependencies[j]
		}
	}
}
