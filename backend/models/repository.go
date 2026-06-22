package models

type Repository struct {
	ID            UUID   `db:"id" json:"id"`
	ExternalID    string `db:"external_id" json:"external_id"`
	Name          string `db:"name" json:"name"`
	Owner         string `db:"owner" json:"owner"`
	DefaultBranch string `db:"default_branch" json:"default_branch"`
	WebURL        string `db:"web_url" json:"web_url"`
	Timestamps
	UpdatedTimestamp
}
