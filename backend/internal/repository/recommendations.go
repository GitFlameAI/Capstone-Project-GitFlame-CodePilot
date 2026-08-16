package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"gitflame-codepilot/backend/internal/domain"
)

// NormalizeRecommendations establishes the storage invariant that one analysis
// run contains at most one card for a deterministic analyzer finding. The hash
// fallback keeps the backend compatible with older recommendation-service
// versions that did not return finding_fingerprint yet.
func NormalizeRecommendations(cards []domain.RecommendationCard) []domain.RecommendationCard {
	normalized := make([]domain.RecommendationCard, 0, len(cards))
	seen := make(map[string]struct{}, len(cards))
	for _, card := range cards {
		fingerprint := strings.ToLower(strings.TrimSpace(card.FindingFingerprint))
		if !validRecommendationFingerprint(fingerprint) {
			fingerprint = recommendationContentFingerprint(card)
		}
		if _, duplicate := seen[fingerprint]; duplicate {
			continue
		}
		seen[fingerprint] = struct{}{}
		card.FindingFingerprint = fingerprint
		normalized = append(normalized, card)
	}
	return normalized
}

func validRecommendationFingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func recommendationContentFingerprint(card domain.RecommendationCard) string {
	line := ""
	if card.Line != nil {
		line = strconv.Itoa(*card.Line)
	}
	identity := strings.Join([]string{
		normalizeRecommendationText(card.Category),
		strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(card.File), "\\", "/"), "./"),
		line,
		normalizeRecommendationText(card.Problem),
		normalizeRecommendationText(card.Suggestion),
	}, "\x1f")
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func normalizeRecommendationText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
