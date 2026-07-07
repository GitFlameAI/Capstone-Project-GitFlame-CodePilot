package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"gitflame-codepilot/backend/internal/domain"
)

type Store interface {
	Ping(context.Context) error
	CreateSession(domain.IssueAnalyzeRequest, domain.AIConfig) (*domain.IssueSession, bool, error)
	Session(string) (*domain.IssueSession, error)
	UpdateSession(*domain.IssueSession) error
	CreateTask(string, string, string) (*domain.AgentTask, error)
	Task(string) (*domain.AgentTask, error)
	LatestTask(string) (*domain.AgentTask, error)
	UpdateTask(*domain.AgentTask) error
	SaveRecommendations(domain.RepositoryMetadata, domain.AIConfig, string, []domain.RecommendationCard) (*domain.RecommendationReport, error)
	Recommendations(string) (*domain.RecommendationReport, error)
	CloseRecommendation(string) (domain.RecommendationCard, error)
	DeleteRecommendation(string) error
	SaveGitFlameConnection(domain.GitFlameConnection) (*domain.GitFlameConnection, error)
	GitFlameConnection(string) (*domain.GitFlameConnection, error)
	SaveGitFlameWebhook(domain.GitFlameWebhookRegistration) (*domain.GitFlameWebhookRegistration, error)
	SaveGitFlameWebhookEvent(domain.GitFlameWebhookEvent) (*domain.GitFlameWebhookEvent, error)
	SaveRepositorySnapshot(domain.RepositorySnapshot, []domain.RepositorySnapshotFile) (*domain.RepositorySnapshot, error)
	RepositorySnapshot(string) (*domain.RepositorySnapshot, []domain.RepositorySnapshotFile, error)
}

var ErrNotFound = errors.New("repository record was not found")

type MemoryStore struct {
	mu            sync.RWMutex
	sessions      map[string]*domain.IssueSession
	issueIndex    map[string]string
	tasks         map[string]*domain.AgentTask
	reports       map[string]*domain.RecommendationReport
	connections   map[string]*domain.GitFlameConnection
	webhooks      map[string]*domain.GitFlameWebhookRegistration
	events        map[string]*domain.GitFlameWebhookEvent
	snapshots     map[string]*domain.RepositorySnapshot
	snapshotFiles map[string][]domain.RepositorySnapshotFile
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: map[string]*domain.IssueSession{}, issueIndex: map[string]string{},
		tasks: map[string]*domain.AgentTask{}, reports: map[string]*domain.RecommendationReport{},
		connections: map[string]*domain.GitFlameConnection{}, webhooks: map[string]*domain.GitFlameWebhookRegistration{},
		events: map[string]*domain.GitFlameWebhookEvent{}, snapshots: map[string]*domain.RepositorySnapshot{},
		snapshotFiles: map[string][]domain.RepositorySnapshotFile{},
	}
}

func (s *MemoryStore) Ping(context.Context) error { return nil }

func (s *MemoryStore) CreateSession(req domain.IssueAnalyzeRequest, cfg domain.AIConfig) (*domain.IssueSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.issueIndex[sessionKey(req.Repository.ID, req.Issue.ID)]; ok {
		return cloneSession(s.sessions[id]), false, nil
	}
	now := time.Now().UTC()
	v := &domain.IssueSession{ID: NewID(), Request: req, Config: cfg, Status: domain.SessionGenerating, CreatedAt: now, UpdatedAt: now}
	s.sessions[v.ID] = cloneSession(v)
	s.issueIndex[req.Issue.ID] = v.ID
	s.issueIndex[sessionKey(req.Repository.ID, req.Issue.ID)] = v.ID
	return cloneSession(v), true, nil
}

func (s *MemoryStore) Session(id string) (*domain.IssueSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if mapped, ok := s.issueIndex[id]; ok {
		id = mapped
	}
	v, ok := s.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneSession(v), nil
}

func (s *MemoryStore) UpdateSession(v *domain.IssueSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.UpdatedAt = time.Now().UTC()
	s.sessions[v.ID] = cloneSession(v)
	s.issueIndex[v.Request.Issue.ID] = v.ID
	s.issueIndex[sessionKey(v.Request.Repository.ID, v.Request.Issue.ID)] = v.ID
	return nil
}

func (s *MemoryStore) CreateTask(sessionID, issueID, taskType string) (*domain.AgentTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	v := &domain.AgentTask{ID: NewID(), SessionID: sessionID, IssueID: issueID, Type: taskType, Status: domain.TaskQueued, Attempt: 1, CreatedAt: now, UpdatedAt: now}
	s.tasks[v.ID] = cloneTask(v)
	return cloneTask(v), nil
}
func (s *MemoryStore) Task(id string) (*domain.AgentTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.tasks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneTask(v), nil
}
func (s *MemoryStore) LatestTask(sessionID string) (*domain.AgentTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *domain.AgentTask
	for _, task := range s.tasks {
		if task.SessionID == sessionID && (latest == nil || task.CreatedAt.After(latest.CreatedAt)) {
			latest = task
		}
	}
	if latest == nil {
		return nil, ErrNotFound
	}
	return cloneTask(latest), nil
}
func (s *MemoryStore) UpdateTask(v *domain.AgentTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.UpdatedAt = time.Now().UTC()
	s.tasks[v.ID] = cloneTask(v)
	return nil
}

func (s *MemoryStore) SaveRecommendations(repository domain.RepositoryMetadata, _ domain.AIConfig, summary string, cards []domain.RecommendationCard) (*domain.RecommendationReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := &domain.RecommendationReport{RepositoryID: repository.ID, Summary: summary, Status: "ready", Recommendations: append([]domain.RecommendationCard(nil), cards...)}
	s.reports[repository.ID] = v
	return cloneReport(v), nil
}
func (s *MemoryStore) Recommendations(id string) (*domain.RecommendationReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.reports[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneReport(v), nil
}
func (s *MemoryStore) CloseRecommendation(id string) (domain.RecommendationCard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.reports {
		for i := range r.Recommendations {
			if r.Recommendations[i].ID == id {
				r.Recommendations[i].State = "closed"
				return r.Recommendations[i], nil
			}
		}
	}
	return domain.RecommendationCard{}, ErrNotFound
}
func (s *MemoryStore) DeleteRecommendation(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.reports {
		for i, c := range r.Recommendations {
			if c.ID == id {
				r.Recommendations = append(r.Recommendations[:i], r.Recommendations[i+1:]...)
				return nil
			}
		}
	}
	return ErrNotFound
}

func (s *MemoryStore) SaveGitFlameConnection(v domain.GitFlameConnection) (*domain.GitFlameConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if v.ID == "" {
		v.ID = NewID()
		v.CreatedAt = now
	} else if existing, ok := s.connections[v.ID]; ok {
		v.CreatedAt = existing.CreatedAt
	}
	if v.TokenStatus == "" {
		v.TokenStatus = "active"
	}
	if v.DefaultBranch == "" {
		v.DefaultBranch = v.Repository.DefaultBranch
	}
	if v.Repository.DefaultBranch == "" {
		v.Repository.DefaultBranch = v.DefaultBranch
	}
	v.UpdatedAt = now
	s.connections[v.ID] = cloneConnection(&v)
	return cloneConnection(&v), nil
}

func (s *MemoryStore) GitFlameConnection(id string) (*domain.GitFlameConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.connections[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneConnection(v), nil
}

func (s *MemoryStore) SaveGitFlameWebhook(v domain.GitFlameWebhookRegistration) (*domain.GitFlameWebhookRegistration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if v.ID == "" {
		v.ID = NewID()
		v.CreatedAt = now
	} else if existing, ok := s.webhooks[v.ID]; ok {
		v.CreatedAt = existing.CreatedAt
	}
	if v.Status == "" {
		v.Status = "pending"
	}
	v.UpdatedAt = now
	s.webhooks[v.ID] = cloneWebhook(&v)
	return cloneWebhook(&v), nil
}

func (s *MemoryStore) SaveGitFlameWebhookEvent(v domain.GitFlameWebhookEvent) (*domain.GitFlameWebhookEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == "" {
		v.ID = NewID()
	}
	if v.Status == "" {
		v.Status = "received"
	}
	if v.ReceivedAt.IsZero() {
		v.ReceivedAt = time.Now().UTC()
	}
	s.events[v.ID] = cloneWebhookEvent(&v)
	return cloneWebhookEvent(&v), nil
}

func (s *MemoryStore) SaveRepositorySnapshot(v domain.RepositorySnapshot, files []domain.RepositorySnapshotFile) (*domain.RepositorySnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == "" {
		v.ID = NewID()
	}
	if v.Status == "" {
		v.Status = "fetched"
	}
	if strings.TrimSpace(v.RepositoryID) == "" {
		return nil, errors.New("repository snapshot requires repository id")
	}
	if v.FetchedAt.IsZero() {
		v.FetchedAt = time.Now().UTC()
	}
	if v.FileCount == 0 {
		v.FileCount = len(files)
	}
	s.snapshots[v.ID] = cloneSnapshot(&v)
	s.snapshotFiles[v.ID] = append([]domain.RepositorySnapshotFile(nil), files...)
	return cloneSnapshot(&v), nil
}

func (s *MemoryStore) RepositorySnapshot(id string) (*domain.RepositorySnapshot, []domain.RepositorySnapshotFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.snapshots[id]
	if !ok {
		return nil, nil, ErrNotFound
	}
	return cloneSnapshot(v), append([]domain.RepositorySnapshotFile(nil), s.snapshotFiles[id]...), nil
}

func cloneSession(v *domain.IssueSession) *domain.IssueSession {
	c := *v
	c.Request.RepositoryFiles = append([]domain.RepositoryFile(nil), v.Request.RepositoryFiles...)
	c.Request.RepositoryContext = append([]string(nil), v.Request.RepositoryContext...)
	c.FeedbackHistory = append([]string(nil), v.FeedbackHistory...)
	if v.GeneratedFiles != nil {
		generated := *v.GeneratedFiles
		generated.Files = append([]domain.GeneratedFileOperation(nil), v.GeneratedFiles.Files...)
		c.GeneratedFiles = &generated
	}
	return &c
}
func cloneTask(v *domain.AgentTask) *domain.AgentTask {
	c := *v
	c.RelevantFiles = append([]domain.RelevantFile(nil), v.RelevantFiles...)
	if v.Error != nil {
		e := *v.Error
		c.Error = &e
	}
	return &c
}
func cloneReport(v *domain.RecommendationReport) *domain.RecommendationReport {
	c := *v
	c.Recommendations = append([]domain.RecommendationCard(nil), v.Recommendations...)
	return &c
}

func cloneConnection(v *domain.GitFlameConnection) *domain.GitFlameConnection {
	c := *v
	return &c
}

func cloneWebhook(v *domain.GitFlameWebhookRegistration) *domain.GitFlameWebhookRegistration {
	c := *v
	c.Events = append([]string(nil), v.Events...)
	return &c
}

func cloneWebhookEvent(v *domain.GitFlameWebhookEvent) *domain.GitFlameWebhookEvent {
	c := *v
	if v.Payload != nil {
		c.Payload = map[string]any{}
		for key, value := range v.Payload {
			c.Payload[key] = value
		}
	}
	if v.Error != nil {
		e := *v.Error
		c.Error = &e
	}
	return &c
}

func cloneSnapshot(v *domain.RepositorySnapshot) *domain.RepositorySnapshot {
	c := *v
	if v.Error != nil {
		e := *v.Error
		c.Error = &e
	}
	return &c
}

func sessionKey(repositoryID, issueID string) string { return repositoryID + "\x00" + issueID }
