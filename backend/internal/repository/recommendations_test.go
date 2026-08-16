package repository

import (
	"strings"
	"testing"

	"gitflame-codepilot/backend/internal/domain"
)

func TestNormalizeRecommendationsDeduplicatesFindingFingerprint(t *testing.T) {
	line := 12
	fingerprint := strings.Repeat("a", 64)
	cards := []domain.RecommendationCard{
		{
			FindingFingerprint: fingerprint,
			Category:           "security",
			File:               "src/app.go",
			Line:               &line,
			Problem:            "first wording",
			Suggestion:         "first fix",
		},
		{
			FindingFingerprint: fingerprint,
			Category:           "security",
			File:               "src/app.go",
			Line:               &line,
			Problem:            "different wording for the same finding",
			Suggestion:         "different fix",
		},
	}
	normalized := NormalizeRecommendations(cards)

	if len(normalized) != 1 {
		t.Fatalf("len(normalized) = %d, want 1", len(normalized))
	}
}

func TestNormalizeRecommendationsBackfillsLegacyFingerprint(t *testing.T) {
	line := 7
	cards := []domain.RecommendationCard{
		{Category: "maintainability", File: "src/app.go", Line: &line, Problem: "  Repeated   code ", Suggestion: "Extract helper"},
		{Category: "MAINTAINABILITY", File: "src/app.go", Line: &line, Problem: "repeated code", Suggestion: "extract helper"},
	}

	normalized := NormalizeRecommendations(cards)

	if len(normalized) != 1 {
		t.Fatalf("len(normalized) = %d, want 1", len(normalized))
	}
	if len(normalized[0].FindingFingerprint) != 64 {
		t.Fatalf("fingerprint = %q", normalized[0].FindingFingerprint)
	}
}
