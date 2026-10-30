package domain

import "time"

type Task struct {
	ID          int64      `json:"id"`
	TeamID      int64      `json:"team_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	CreatedBy   int64      `json:"created_by"`
	AssigneeID  *int64     `json:"assignee_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Version     int64      `json:"version"`
}

type TaskFilter struct {
	TeamID     int64
	Status     *TaskStatus
	AssigneeID *int64
	Limit      int
	Offset     int
}

type TaskList struct {
	Items  []Task `json:"items"`
	Total  int64  `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}
