package service

import (
	"fmt"
	"strings"
	"testing"

	"gitflame-codepilot/backend/internal/domain"
)

func TestValidateRepositoryFilesForIntegrationRejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	content := strings.Repeat("x", 500_000)
	files := make([]domain.RepositoryFile, 41)
	for index := range files {
		files[index] = domain.RepositoryFile{
			Path:    fmt.Sprintf("src/file-%02d.txt", index),
			Content: content,
		}
	}

	err := ValidateRepositoryFilesForIntegration(files)
	if err == nil || !strings.Contains(err.Error(), "payload exceeds") {
		t.Fatalf("error = %v, want payload limit error", err)
	}
}
