package domain

import "time"

// FieldChange holds a task field's old and new values.
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

type TaskHistory struct {
	ID        int64                  `json:"id"`
	TaskID    int64                  `json:"task_id"`
	ChangedBy int64                  `json:"changed_by"`
	Changes   map[string]FieldChange `json:"changes"`
	CreatedAt time.Time              `json:"created_at"`
}
