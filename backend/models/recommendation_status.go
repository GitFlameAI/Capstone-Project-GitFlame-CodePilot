package models

type RecommendationStatus struct {
	ID               UUID                      `db:"id" json:"id"`
	RecommendationID UUID                      `db:"recommendation_id" json:"recommendation_id"`
	Status           RecommendationStatusValue `db:"status" json:"status"`
	ChangedBy        string                    `db:"changed_by" json:"changed_by"`
	Timestamps
}
