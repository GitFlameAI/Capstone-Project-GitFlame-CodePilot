package models

type PlanRevision struct {
	ID                 UUID               `db:"id" json:"id"`
	GeneratedPlanID    UUID               `db:"generated_plan_id" json:"generated_plan_id"`
	IssueSessionID     UUID               `db:"issue_session_id" json:"issue_session_id"`
	AgentTaskID        *UUID              `db:"agent_task_id" json:"agent_task_id,omitempty"`
	RevisionNumber     int                `db:"revision_number" json:"revision_number"`
	PlanMarkdown       string             `db:"plan_markdown" json:"plan_markdown"`
	CorrectionFeedback string             `db:"correction_feedback" json:"correction_feedback"`
	Source             PlanRevisionSource `db:"source" json:"source"`
	Timestamps
}
