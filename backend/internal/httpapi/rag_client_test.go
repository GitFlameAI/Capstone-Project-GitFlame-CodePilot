package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRAGClientReportsIndexTimeoutSeparatelyFromUnreachable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	client := NewRAGClient(server.URL, "", 10*time.Millisecond)
	_, err := client.Index(context.Background(), RAGIndexRequest{
		RepositoryID: "acme/project",
		CommitSHA:    "abc123",
	})
	var integration *IntegrationError
	if !errors.As(err, &integration) {
		t.Fatalf("error = %v, want IntegrationError", err)
	}
	if integration.Code != "rag_index_timeout" || integration.Status != http.StatusGatewayTimeout {
		t.Fatalf("integration error = %#v, want timeout/504", integration)
	}
}
