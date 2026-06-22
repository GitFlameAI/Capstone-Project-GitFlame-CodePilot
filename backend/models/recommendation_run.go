package models

import "time"

type RecommendationRun struct {
	ID            UUID                    `db:"id" json:"id"`
	RepositoryID  UUID                    `db:"repository_id" json:"repository_id"`
	AIConfigID    UUID                    `db:"ai_config_id" json:"ai_config_id"`
	Summary       string                  `db:"summary" json:"summary"`
	Status        RecommendationRunStatus `db:"status" json:"status"`
	RetentionDays int                     `db:"retention_days" json:"retention_days"`
	ExpiresAt     time.Time               `db:"expires_at" json:"expires_at"`
	Timestamps
	UpdatedTimestamp
}
