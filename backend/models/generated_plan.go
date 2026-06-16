package models

type GeneratedPlan struct {
	ID             UUID   `db:"id" json:"id"`
	IssueSessionID UUID   `db:"issue_session_id" json:"issue_session_id"`
	PlanMarkdown   string `db:"plan_markdown" json:"plan_markdown"`
	Timestamps
}
