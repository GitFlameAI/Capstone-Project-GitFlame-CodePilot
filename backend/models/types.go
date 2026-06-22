package models

import "time"

// UUID is used as a placeholder for PostgreSQL UUID columns.
// The storage layer can later replace it with a driver-specific UUID type.
type UUID string

type IssueSessionStatus string

const (
	IssueSessionStatusCreated       IssueSessionStatus = "created"
	IssueSessionStatusQueued        IssueSessionStatus = "queued"
	IssueSessionStatusProcessing    IssueSessionStatus = "processing"
	IssueSessionStatusPlanGenerated IssueSessionStatus = "plan_generated"
	IssueSessionStatusApproved      IssueSessionStatus = "approved"
	IssueSessionStatusCorrection    IssueSessionStatus = "correction_requested"
	IssueSessionStatusRejected      IssueSessionStatus = "rejected"
	IssueSessionStatusFailed        IssueSessionStatus = "failed"
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

type AgentTaskType string

const (
	AgentTaskInitialPlan            AgentTaskType = "initial_plan"
	AgentTaskPlanRevision           AgentTaskType = "plan_revision"
	AgentTaskRecommendationAnalysis AgentTaskType = "recommendation_analysis"
)

type AgentTaskStatus string

const (
	AgentTaskQueued     AgentTaskStatus = "queued"
	AgentTaskProcessing AgentTaskStatus = "processing"
	AgentTaskCompleted  AgentTaskStatus = "completed"
	AgentTaskFailed     AgentTaskStatus = "failed"
)

type PlanRevisionSource string

const (
	PlanRevisionInitial    PlanRevisionSource = "initial"
	PlanRevisionCorrection PlanRevisionSource = "correction"
	PlanRevisionRetry      PlanRevisionSource = "retry"
)

type Timestamps struct {
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type UpdatedTimestamp struct {
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
