package models

type AgentTaskStatusHistory struct {
	ID          UUID            `db:"id" json:"id"`
	AgentTaskID UUID            `db:"agent_task_id" json:"agent_task_id"`
	Status      AgentTaskStatus `db:"status" json:"status"`
	Message     string          `db:"message" json:"message"`
	Timestamps
}
