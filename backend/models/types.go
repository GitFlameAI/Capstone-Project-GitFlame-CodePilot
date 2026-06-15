package models

import "time"

// UUID is used as a placeholder for PostgreSQL UUID columns.
// The storage layer can later replace it with a driver-specific UUID type.
type UUID string

type IssueSessionStatus string

const (
	IssueSessionStatusCreated       IssueSessionStatus = "created"
	IssueSessionStatusPlanGenerated IssueSessionStatus = "plan_generated"
	IssueSessionStatusApproved      IssueSessionStatus = "approved"
	IssueSessionStatusCorrection    IssueSessionStatus = "correction_requested"
	IssueSessionStatusRejected      IssueSessionStatus = "rejected"
)

type UserResponseType string

const (
	UserResponseApprove UserResponseType = "approve"
	UserResponseCorrect UserResponseType = "correct"
	UserResponseReject  UserResponseType = "reject"
)

type RecommendationRunStatus string

const (
	RecommendationRunStatusPending   RecommendationRunStatus = "pending"
	RecommendationRunStatusCompleted RecommendationRunStatus = "completed"
	RecommendationRunStatusFailed    RecommendationRunStatus = "failed"
)

type RecommendationStatusValue string

const (
	RecommendationStatusOpen    RecommendationStatusValue = "open"
	RecommendationStatusClosed  RecommendationStatusValue = "closed"
	RecommendationStatusDeleted RecommendationStatusValue = "deleted"
)

type Timestamps struct {
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
