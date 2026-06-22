package models

type IssueSession struct {
	ID              UUID               `db:"id" json:"id"`
	RepositoryID    UUID               `db:"repository_id" json:"repository_id"`
	AIConfigID      UUID               `db:"ai_config_id" json:"ai_config_id"`
	ExternalIssueID string             `db:"external_issue_id" json:"external_issue_id"`
	IssueTitle      string             `db:"issue_title" json:"issue_title"`
	IssueBody       string             `db:"issue_body" json:"issue_body"`
	IssueAuthor     string             `db:"issue_author" json:"issue_author"`
	Status          IssueSessionStatus `db:"status" json:"status"`
	CurrentRevision int                `db:"current_revision" json:"current_revision"`
	Timestamps
	UpdatedTimestamp
}
