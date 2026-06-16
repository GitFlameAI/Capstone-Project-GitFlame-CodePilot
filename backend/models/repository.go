package models

type Repository struct {
	ID            UUID   `db:"id" json:"id"`
	Name          string `db:"name" json:"name"`
	Owner         string `db:"owner" json:"owner"`
	DefaultBranch string `db:"default_branch" json:"default_branch"`
	Timestamps
}
