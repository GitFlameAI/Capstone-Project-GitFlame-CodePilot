package models

import "encoding/json"

type AIConfig struct {
	ID               UUID            `db:"id" json:"id"`
	RepositoryID     UUID            `db:"repository_id" json:"repository_id"`
	RawYML           string          `db:"raw_yml" json:"raw_yml"`
	ParsedConfigJSON json.RawMessage `db:"parsed_config_json" json:"parsed_config_json"`
	IsValid          bool            `db:"is_valid" json:"is_valid"`
	Timestamps
}
