package httpapi

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Repository indexing (CodeRAG) is by far the slowest step in the product: it
// reads every file of the repository from GitFlame and embeds it. Running that
// inside the HTTP request that connects a repository made the user wait minutes
// on a blank screen, so indexing now runs in the background and the request
// returns as soon as the connection is stored. Work that actually needs the
// index (plan generation, recommendation analysis) waits for the running job
// first, which hides the cost without ever using a stale index.
//
// Job state is kept in memory, per backend process, on purpose:
//   - the goroutine that runs the job lives in this process, so a database row
//     would only mirror what this registry already knows;
//   - a "running" row left behind by a restart would have to be expired by a
//     timeout anyway, while an in-memory registry simply starts empty and the
//     next request re-verifies the index against CodeRAG.
//
// The trade-off is that this assumes a single backend instance, which is what
// docker-compose deploys. Scaling the backend horizontally requires moving this
// registry into Postgres; see docs/OPERATIONS.md.
const (
	indexStatusRunning   = "running"
	indexStatusCompleted = "completed"
	indexStatusFailed    = "failed"
	indexStatusIdle      = "idle"
	indexStatusDisabled  = "disabled"

	// indexJobResultTTL is how long a successful job is reused instead of
	// starting a new one. Browsing the repository asks for an index check on
	// every call; without this window every page view would re-verify the index
	// against CodeRAG and GitFlame.
	indexJobResultTTL = time.Minute

	// indexJobErrorLimit caps the stored error detail. It is shown to humans,
	// so a verbose upstream message must not bloat the status payload.
	indexJobErrorLimit = 300

	// defaultIndexWaitTimeout bounds how long a request may wait for a running
	// background job before it gives up and asks the caller to retry.
	defaultIndexWaitTimeout = 10 * time.Minute
)

// errIndexWaitTimeout is returned when a caller stopped waiting for a running
// job. The job itself keeps running.
var errIndexWaitTimeout = errors.New("timed out waiting for repository indexing")

// indexJobStatus is the observable state of one repository indexing job. It is
// returned by GET /integrations/gitflame/connections/{id}/index and embedded in
// the repository tree and files responses.
type indexJobStatus struct {
	RepositoryID   string     `json:"repository_id"`
	ConnectionID   string     `json:"connection_id,omitempty"`
	Ref            string     `json:"ref,omitempty"`
	CommitSHA      string     `json:"commit_sha,omitempty"`
	Status         string     `json:"status"`
	ErrorCode      string     `json:"error_code,omitempty"`
	Error          string     `json:"error,omitempty"`
	FileCount      int        `json:"file_count,omitempty"`
	ChunkCount     int        `json:"chunk_count,omitempty"`
	EmbeddingCount int        `json:"embedding_count,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	DurationMS     int64      `json:"duration_ms,omitempty"`
}

type indexJob struct {
	status indexJobStatus
	done   chan struct{}
}

type indexJobRegistry struct {
	mu   sync.Mutex
	jobs map[string]*indexJob
}

func newIndexJobRegistry() *indexJobRegistry {
	return &indexJobRegistry{jobs: map[string]*indexJob{}}
}

// begin reserves the indexing slot for a repository. The second return value
// reports whether the caller now owns the job and must run it: a job that is
// already running, or a forced job's recently finished predecessor, is reused
// instead of being started twice.
func (r *indexJobRegistry) begin(repositoryID, connectionID, ref string, force bool) (indexJobStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.jobs[repositoryID]; ok {
		if existing.status.Status == indexStatusRunning {
			return existing.status, false
		}
		if !force && existing.status.Status == indexStatusCompleted &&
			existing.status.FinishedAt != nil &&
			time.Since(*existing.status.FinishedAt) < indexJobResultTTL {
			return existing.status, false
		}
	}
	started := time.Now().UTC()
	job := &indexJob{
		status: indexJobStatus{
			RepositoryID: repositoryID,
			ConnectionID: connectionID,
			Ref:          ref,
			Status:       indexStatusRunning,
			StartedAt:    &started,
		},
		done: make(chan struct{}),
	}
	r.jobs[repositoryID] = job
	return job.status, true
}

// finish records the outcome of the job owned by the caller and releases every
// waiter. Calling it for a job that is no longer running is a no-op, so a late
// goroutine cannot overwrite a newer job's state.
func (r *indexJobRegistry) finish(repositoryID string, result RAGIndexResult, cause error) indexJobStatus {
	r.mu.Lock()
	job, ok := r.jobs[repositoryID]
	if !ok || job.status.Status != indexStatusRunning {
		r.mu.Unlock()
		return indexJobStatus{}
	}
	finished := time.Now().UTC()
	job.status.FinishedAt = &finished
	if job.status.StartedAt != nil {
		job.status.DurationMS = finished.Sub(*job.status.StartedAt).Milliseconds()
	}
	if cause != nil {
		job.status.Status = indexStatusFailed
		job.status.ErrorCode = integrationCode(cause)
		job.status.Error = truncateDetail(cause.Error(), indexJobErrorLimit)
	} else {
		job.status.Status = indexStatusCompleted
		if result.CommitSHA != "" {
			job.status.CommitSHA = result.CommitSHA
		}
		job.status.FileCount = result.FileCount
		job.status.ChunkCount = result.ChunkCount
		job.status.EmbeddingCount = result.EmbeddingCount
	}
	status := job.status
	r.mu.Unlock()
	close(job.done)
	return status
}

// status returns the last known state for a repository. The second return value
// is false when this process has never indexed the repository.
func (r *indexJobRegistry) status(repositoryID string) (indexJobStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[repositoryID]
	if !ok {
		return indexJobStatus{}, false
	}
	return job.status, true
}

// wait blocks until the running job for a repository finishes. It returns
// immediately when no job is known (false) or when the job already finished.
// The error is errIndexWaitTimeout when the wait budget ran out, or the request
// context error when the caller disconnected; in both cases the job continues.
func (r *indexJobRegistry) wait(ctx context.Context, repositoryID string, timeout time.Duration) (indexJobStatus, bool, error) {
	r.mu.Lock()
	job, ok := r.jobs[repositoryID]
	if !ok {
		r.mu.Unlock()
		return indexJobStatus{}, false, nil
	}
	if job.status.Status != indexStatusRunning {
		status := job.status
		r.mu.Unlock()
		return status, true, nil
	}
	done := job.done
	r.mu.Unlock()

	if timeout <= 0 {
		timeout = defaultIndexWaitTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		status, _ := r.status(repositoryID)
		return status, true, nil
	case <-ctx.Done():
		return indexJobStatus{}, true, ctx.Err()
	case <-timer.C:
		return indexJobStatus{}, true, errIndexWaitTimeout
	}
}

// runningCount is read by the metrics collector.
func (r *indexJobRegistry) runningCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	running := 0
	for _, job := range r.jobs {
		if job.status.Status == indexStatusRunning {
			running++
		}
	}
	return running
}

func truncateDetail(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

// WaitForBackgroundWork blocks until no indexing job is running or the context
// expires, and returns how many jobs were still running when it gave up. It is
// used during shutdown so a redeploy does not routinely throw away several
// minutes of indexing.
func (s *Server) WaitForBackgroundWork(ctx context.Context) int {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		running := s.indexJobs.runningCount()
		if running == 0 {
			return 0
		}
		select {
		case <-ctx.Done():
			return running
		case <-ticker.C:
		}
	}
}
