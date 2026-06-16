package app

import (
	"sync"
	"time"
)

const (
	statusPlanGenerated       = "plan_generated"
	statusApproved            = "approved"
	statusCorrectionRequested = "correction_requested"
	statusRejected            = "rejected"

	recommendationOpen   = "open"
	recommendationClosed = "closed"
)

type IssueSession struct {
	SessionID       string
	Request         IssueAnalyzeRequest
	Config          AIConfig
	PlanMarkdown    string
	Status          string
	Revision        int
	GitWorkflow     *GitWorkflowResponse
	FeedbackHistory []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RecommendationReport struct {
	RepositoryID    string
	Summary         string
	Recommendations []RecommendationCard
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type MemoryStore struct {
	mu                    sync.RWMutex
	issueSessions         map[string]*IssueSession
	issueIndex            map[string]string
	recommendationReports map[string]*RecommendationReport
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		issueSessions:         make(map[string]*IssueSession),
		issueIndex:            make(map[string]string),
		recommendationReports: make(map[string]*RecommendationReport),
	}
}

func (s *MemoryStore) SaveIssueSession(request IssueAnalyzeRequest, cfg AIConfig, planMarkdown string) *IssueSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	session := &IssueSession{
		SessionID:    newID(),
		Request:      request,
		Config:       cfg,
		PlanMarkdown: planMarkdown,
		Status:       statusPlanGenerated,
		Revision:     1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.issueSessions[session.SessionID] = session
	s.issueIndex[request.Issue.ID] = session.SessionID
	return cloneIssueSession(session)
}

func (s *MemoryStore) GetIssueSession(issueOrSessionID string) (*IssueSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if session, ok := s.issueSessions[issueOrSessionID]; ok {
		return cloneIssueSession(session), true
	}
	sessionID, ok := s.issueIndex[issueOrSessionID]
	if !ok {
		return nil, false
	}
	session, ok := s.issueSessions[sessionID]
	if !ok {
		return nil, false
	}
	return cloneIssueSession(session), true
}

func (s *MemoryStore) UpdateIssueSession(session *IssueSession) *IssueSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	session.UpdatedAt = time.Now().UTC()
	s.issueSessions[session.SessionID] = cloneIssueSession(session)
	s.issueIndex[session.Request.Issue.ID] = session.SessionID
	return cloneIssueSession(session)
}

func (s *MemoryStore) SaveRecommendations(repositoryID, summary string, cards []RecommendationCard) *RecommendationReport {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	report := &RecommendationReport{
		RepositoryID:    repositoryID,
		Summary:         summary,
		Recommendations: append([]RecommendationCard(nil), cards...),
		Status:          "ready",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.recommendationReports[repositoryID] = report
	return cloneRecommendationReport(report)
}

func (s *MemoryStore) GetRecommendationReport(repositoryID string) (*RecommendationReport, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report, ok := s.recommendationReports[repositoryID]
	if !ok {
		return nil, false
	}
	return cloneRecommendationReport(report), true
}

func (s *MemoryStore) CloseRecommendation(recommendationID string) (RecommendationCard, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, report := range s.recommendationReports {
		for i := range report.Recommendations {
			if report.Recommendations[i].ID == recommendationID {
				report.Recommendations[i].State = recommendationClosed
				report.UpdatedAt = time.Now().UTC()
				return report.Recommendations[i], true
			}
		}
	}
	return RecommendationCard{}, false
}

func (s *MemoryStore) DeleteRecommendation(recommendationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, report := range s.recommendationReports {
		next := report.Recommendations[:0]
		deleted := false
		for _, card := range report.Recommendations {
			if card.ID == recommendationID {
				deleted = true
				continue
			}
			next = append(next, card)
		}
		if deleted {
			report.Recommendations = next
			report.UpdatedAt = time.Now().UTC()
			return true
		}
	}
	return false
}

func cloneIssueSession(session *IssueSession) *IssueSession {
	clone := *session
	clone.Request.RepositoryContext = append([]string(nil), session.Request.RepositoryContext...)
	clone.FeedbackHistory = append([]string(nil), session.FeedbackHistory...)
	return &clone
}

func cloneRecommendationReport(report *RecommendationReport) *RecommendationReport {
	clone := *report
	clone.Recommendations = append([]RecommendationCard(nil), report.Recommendations...)
	return &clone
}
