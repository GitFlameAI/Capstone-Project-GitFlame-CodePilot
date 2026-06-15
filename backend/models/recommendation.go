package models

type Recommendation struct {
	ID                  UUID                      `db:"id" json:"id"`
	RecommendationRunID UUID                      `db:"recommendation_run_id" json:"recommendation_run_id"`
	FilePath            string                    `db:"file_path" json:"file_path"`
	LineNumber          *int                      `db:"line_number" json:"line_number,omitempty"`
	Category            string                    `db:"category" json:"category"`
	Severity            string                    `db:"severity" json:"severity"`
	Problem             string                    `db:"problem" json:"problem"`
	Suggestion          string                    `db:"suggestion" json:"suggestion"`
	CurrentStatus       RecommendationStatusValue `db:"current_status" json:"current_status"`
	Timestamps
}
