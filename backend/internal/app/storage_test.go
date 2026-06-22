package app

import (
	"context"
	"strings"
	"testing"
)

func TestMemoryStoreTracksMultipleCorrectionsForOneIssue(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	request := IssueAnalyzeRequest{
		Repository: RepositoryMetadata{
			ID:            "demo-repo",
			Name:          "codepilot-demo",
			DefaultBranch: "main",
		},
		Issue: IssuePayload{
			ID:     "ISSUE-69",
			Title:  "Persist correction history",
			Body:   "Store several correction rounds for one issue.",
			Author: "amir",
		},
		YAMLConfig: "version: 1\nrecommendations:\n  retention_days: 30\n",
	}
	cfg := AIConfig{
		Raw:            request.YAMLConfig,
		Version:        "1",
		DefaultBranch:  "main",
		ApproveCommand: "/approve",
		CorrectCommand: "/correct",
		RejectCommand:  "/reject",
		RetentionDays:  30,
	}

	session, err := store.SaveIssueSession(ctx, request, cfg, "# Plan\n\nInitial plan.")
	if err != nil {
		t.Fatalf("save initial session: %v", err)
	}

	feedbacks := []string{
		"Add revision history verification.",
		"Also cover task state persistence.",
	}
	for _, feedback := range feedbacks {
		session.FeedbackHistory = append(session.FeedbackHistory, feedback)
		session.Revision++
		session.Status = statusCorrectionRequested
		session.PlanMarkdown += "\n\n## Revision feedback\n- " + feedback
		updated, err := store.UpdateIssueSession(ctx, session)
		if err != nil {
			t.Fatalf("update correction %q: %v", feedback, err)
		}
		session = updated
	}

	loaded, ok, err := store.GetIssueSession(ctx, "ISSUE-69")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if !ok {
		t.Fatal("expected issue session to be found")
	}
	if loaded.Revision != 3 {
		t.Fatalf("expected revision 3, got %d", loaded.Revision)
	}
	if len(loaded.FeedbackHistory) != 2 {
		t.Fatalf("expected 2 feedback entries, got %d", len(loaded.FeedbackHistory))
	}
	for _, feedback := range feedbacks {
		if !strings.Contains(loaded.PlanMarkdown, feedback) {
			t.Fatalf("expected plan markdown to contain feedback %q", feedback)
		}
	}
}
