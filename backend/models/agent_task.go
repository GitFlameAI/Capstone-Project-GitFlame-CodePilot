package models

import "time"

type AgentTask struct {
	ID                   UUID            `db:"id" json:"id"`
	IssueSessionID       *UUID           `db:"issue_session_id" json:"issue_session_id,omitempty"`
	GeneratedPlanID      *UUID           `db:"generated_plan_id" json:"generated_plan_id,omitempty"`
	TaskType             AgentTaskType   `db:"task_type" json:"task_type"`
	Status               AgentTaskStatus `db:"status" json:"status"`
	ErrorMessage         string          `db:"error_message" json:"error_message"`
	ToolExecutionSummary string          `db:"tool_execution_summary" json:"tool_execution_summary"`
	StartedAt            *time.Time      `db:"started_at" json:"started_at,omitempty"`
	CompletedAt          *time.Time      `db:"completed_at" json:"completed_at,omitempty"`
	Timestamps
	UpdatedTimestamp
}
