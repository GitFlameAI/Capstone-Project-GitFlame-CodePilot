package models

type UserResponse struct {
	ID             UUID             `db:"id" json:"id"`
	IssueSessionID UUID             `db:"issue_session_id" json:"issue_session_id"`
	ResponseType   UserResponseType `db:"response_type" json:"response_type"`
	Message        string           `db:"message" json:"message"`
	Author         string           `db:"author" json:"author"`
	Timestamps
}
