// Package domain содержит сущности, ошибки и правила доступа.
// Пакет не зависит от инфраструктуры — его можно тестировать без БД.
package domain

import (
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrForbidden     = errors.New("forbidden")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
	ErrValidation    = errors.New("validation error")
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

func (r Role) Valid() bool {
	return r == RoleOwner || r == RoleAdmin || r == RoleMember
}

// IsManager — создатель или администратор команды.
func (r Role) IsManager() bool {
	return r == RoleOwner || r == RoleAdmin
}

type TaskStatus string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
)

func (s TaskStatus) Valid() bool {
	return s == StatusTodo || s == StatusInProgress || s == StatusDone
}

type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
}

type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	// Role — роль текущего пользователя в команде.
	Role Role `json:"role,omitempty"`
}

type TeamMember struct {
	TeamID int64 `json:"team_id"`
	UserID int64 `json:"user_id"`
	Role   Role  `json:"role"`
}

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

// FieldChange — старое и новое значение поля задачи.
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

type TaskComment struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
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

type AssigneeStat struct {
	UserID      int64  `json:"user_id"`
	Name        string `json:"name"`
	ClosedTasks int64  `json:"closed_tasks"`
}

type TaskCommentStat struct {
	TaskID   int64  `json:"task_id"`
	Title    string `json:"title"`
	Comments int64  `json:"comments"`
}

type TeamStats struct {
	TeamID              int64            `json:"team_id"`
	TasksByStatus       map[string]int64 `json:"tasks_by_status"`
	TopAssignees30d     []AssigneeStat   `json:"top_assignees_30d"`
	ClosedTasks         int64            `json:"closed_tasks"`
	AvgCloseTimeSeconds *float64         `json:"avg_close_time_seconds"`
	TotalComments       int64            `json:"total_comments"`
	// CommentsByTask — до 10 задач с наибольшим числом комментариев.
	CommentsByTask []TaskCommentStat `json:"comments_by_task"`
}
