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
	UpsertAppUser(domain.AppUser) (*domain.AppUser, error)
	CreateAppSession(string, []byte, time.Time) (*domain.AppSession, error)
	AppSessionByTokenHash([]byte) (*domain.AppSession, error)
	RevokeAppSession(string) error
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
	UserGitFlameConnection(string, string) (*domain.GitFlameConnection, error)
	UserGitFlameConnectionByRepository(string, string) (*domain.GitFlameConnection, error)
	RevokeGitFlameConnection(string, string) (*domain.GitFlameConnection, error)
	TouchGitFlameConnection(string, string) error
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
	users         map[string]*domain.AppUser
	userIndex     map[string]string
	appSessions   map[string]*domain.AppSession
	sessionHashes map[string]string
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
		users: map[string]*domain.AppUser{}, userIndex: map[string]string{},
		appSessions: map[string]*domain.AppSession{}, sessionHashes: map[string]string{},
		connections: map[string]*domain.GitFlameConnection{}, webhooks: map[string]*domain.GitFlameWebhookRegistration{},
		events: map[string]*domain.GitFlameWebhookEvent{}, snapshots: map[string]*domain.RepositorySnapshot{},
		snapshotFiles: map[string][]domain.RepositorySnapshotFile{},
	}
}

func (s *MemoryStore) Ping(context.Context) error { return nil }

func (s *MemoryStore) UpsertAppUser(v domain.AppUser) (*domain.AppUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if v.GitFlameUserID == "" {
		return nil, errors.New("gitflame user id is required")
	}
	if strings.TrimSpace(v.Username) == "" {
		v.Username = v.GitFlameUserID
	}
	if id, ok := s.userIndex[v.GitFlameUserID]; ok {
		existing := s.users[id]
		existing.Username = v.Username
		existing.UpdatedAt = now
		return cloneUser(existing), nil
	}
	if v.ID == "" {
		v.ID = NewID()
	}
	v.CreatedAt = now
	v.UpdatedAt = now
	s.users[v.ID] = cloneUser(&v)
	s.userIndex[v.GitFlameUserID] = v.ID
	return cloneUser(&v), nil
}

func (s *MemoryStore) CreateAppSession(userID string, tokenHash []byte, expiresAt time.Time) (*domain.AppSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return nil, ErrNotFound
	}
	now := time.Now().UTC()
	session := &domain.AppSession{ID: NewID(), User: *cloneUser(user), ExpiresAt: expiresAt, CreatedAt: now}
	s.appSessions[session.ID] = cloneAppSession(session)
	s.sessionHashes[string(tokenHash)] = session.ID
	return cloneAppSession(session), nil
}

func (s *MemoryStore) AppSessionByTokenHash(tokenHash []byte) (*domain.AppSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.sessionHashes[string(tokenHash)]
	if !ok {
		return nil, ErrNotFound
	}
	session := s.appSessions[id]
	if session == nil || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrNotFound
	}
	now := time.Now().UTC()
	session.LastSeenAt = &now
	return cloneAppSession(session), nil
}

func (s *MemoryStore) RevokeAppSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.appSessions[sessionID]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	session.RevokedAt = &now
	return nil
}

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
	for _, existing := range s.connections {
		if existing.UserID == v.UserID && existing.Repository.ID == v.Repository.ID && v.Repository.ID != "" {
			v.ID = existing.ID
			v.CreatedAt = existing.CreatedAt
			break
		}
	}
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

func (s *MemoryStore) UserGitFlameConnection(userID, id string) (*domain.GitFlameConnection, error) {
	connection, err := s.GitFlameConnection(id)
	if err != nil {
		return nil, err
	}
	if connection.UserID != userID {
		return nil, ErrNotFound
	}
	return connection, nil
}

func (s *MemoryStore) UserGitFlameConnectionByRepository(userID, repositoryID string) (*domain.GitFlameConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, connection := range s.connections {
		if connection.UserID == userID && connection.Repository.ID == repositoryID && connection.RevokedAt == nil {
			return cloneConnection(connection), nil
		}
	}
	return nil, ErrNotFound
}

func (s *MemoryStore) RevokeGitFlameConnection(userID, id string) (*domain.GitFlameConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	connection, ok := s.connections[id]
	if !ok || connection.UserID != userID {
		return nil, ErrNotFound
	}
	now := time.Now().UTC()
	connection.TokenStatus = "revoked"
	connection.RevokedAt = &now
	connection.UpdatedAt = now
	return cloneConnection(connection), nil
}

func (s *MemoryStore) TouchGitFlameConnection(userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	connection, ok := s.connections[id]
	if !ok || connection.UserID != userID {
		return ErrNotFound
	}
	now := time.Now().UTC()
	connection.LastUsedAt = &now
	connection.UpdatedAt = now
	return nil
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

func cloneUser(v *domain.AppUser) *domain.AppUser {
	c := *v
	return &c
}

func cloneAppSession(v *domain.AppSession) *domain.AppSession {
	c := *v
	c.User = *cloneUser(&v.User)
	if v.LastSeenAt != nil {
		lastSeenAt := *v.LastSeenAt
		c.LastSeenAt = &lastSeenAt
	}
	if v.RevokedAt != nil {
		revokedAt := *v.RevokedAt
		c.RevokedAt = &revokedAt
	}
	return &c
}

func cloneConnection(v *domain.GitFlameConnection) *domain.GitFlameConnection {
	c := *v
	c.Scopes = append([]string(nil), v.Scopes...)
	c.TokenMaterial.Ciphertext = append([]byte(nil), v.TokenMaterial.Ciphertext...)
	c.TokenMaterial.Nonce = append([]byte(nil), v.TokenMaterial.Nonce...)
	if v.TokenExpiresAt != nil {
		tokenExpiresAt := *v.TokenExpiresAt
		c.TokenExpiresAt = &tokenExpiresAt
	}
	if v.LastValidatedAt != nil {
		lastValidatedAt := *v.LastValidatedAt
		c.LastValidatedAt = &lastValidatedAt
	}
	if v.LastUsedAt != nil {
		lastUsedAt := *v.LastUsedAt
		c.LastUsedAt = &lastUsedAt
	}
	if v.RevokedAt != nil {
		revokedAt := *v.RevokedAt
		c.RevokedAt = &revokedAt
	}
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
