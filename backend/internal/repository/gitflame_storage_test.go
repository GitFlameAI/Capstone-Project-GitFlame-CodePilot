package repository

import (
	"testing"

	"gitflame-codepilot/backend/internal/domain"
)

func TestMemoryStorePersistsGitFlameIntegrationStorage(t *testing.T) {
	store := NewMemoryStore()
	connection, err := store.SaveGitFlameConnection(domain.GitFlameConnection{
		Repository: domain.RepositoryMetadata{ID: "repo-1", Name: "repo", DefaultBranch: "main", WebURL: "https://gitflame.test/owner/repo"},
		RepoURL:    "https://gitflame.test/owner/repo", AccessTokenEncrypted: "ciphertext", TokenLast4: "1234",
	})
	if err != nil {
		t.Fatal(err)
	}
	loadedConnection, err := store.GitFlameConnection(connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedConnection.Repository.ID != "repo-1" || loadedConnection.TokenStatus != "active" || loadedConnection.AccessTokenEncrypted != "ciphertext" {
		t.Fatalf("unexpected connection: %+v", loadedConnection)
	}

	webhook, err := store.SaveGitFlameWebhook(domain.GitFlameWebhookRegistration{
		ConnectionID: connection.ID, WebhookURL: "https://codepilot.test/integrations/gitflame/webhooks/issues",
		WebhookSecretHash: "hash", Events: []string{"issue.created", "issue.updated"},
	})
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.SaveGitFlameWebhookEvent(domain.GitFlameWebhookEvent{
		WebhookID: webhook.ID, EventType: "issue.updated", Action: "opened", DeliveryID: "delivery-1",
		RepositoryID: "repo-1", Payload: map[string]any{"issue_id": "42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Status != "received" || event.Payload["issue_id"] != "42" {
		t.Fatalf("unexpected webhook event: %+v", event)
	}

	snapshot, err := store.SaveRepositorySnapshot(domain.RepositorySnapshot{
		RepositoryID: "repo-1", ConnectionID: connection.ID, Ref: "main", CommitSHA: "abc123",
	}, []domain.RepositorySnapshotFile{{Path: "README.md", ContentHash: "hash", CommitSHA: "abc123"}})
	if err != nil {
		t.Fatal(err)
	}
	loadedSnapshot, files, err := store.RepositorySnapshot(snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedSnapshot.FileCount != 1 || len(files) != 1 || files[0].Path != "README.md" {
		t.Fatalf("unexpected snapshot: snapshot=%+v files=%+v", loadedSnapshot, files)
	}
}
