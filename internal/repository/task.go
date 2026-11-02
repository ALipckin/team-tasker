package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"backend/internal/domain"
)

// TaskRepo — интерфейс репозитория задач.
type TaskRepo interface {
	Create(ctx context.Context, t *domain.Task) error
	GetByID(ctx context.Context, id int64) (domain.Task, error)
	// GetByIDForUpdate блокирует строку задачи до конца транзакции (SELECT ... FOR UPDATE).
	GetByIDForUpdate(ctx context.Context, id int64) (domain.Task, error)
	// Update сохраняет задачу, если её версия в БД равна expectedVersion, и увеличивает версию.
	// Если версия не совпала — ErrConflict.
	Update(ctx context.Context, t *domain.Task, expectedVersion int64) error
	List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, int64, error)
}

type taskRepo struct {
	db DB
}

func NewTaskRepo(db DB) TaskRepo {
	return &taskRepo{db: db}
}

const taskColumns = `id, team_id, title, description, status, created_by, assignee_id,
	created_at, updated_at, closed_at, version`

func (r *taskRepo) Create(ctx context.Context, t *domain.Task) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx, `
		INSERT INTO tasks (team_id, title, description, status, created_by, assignee_id, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.TeamID, t.Title, t.Description, t.Status, t.CreatedBy, t.AssigneeID, t.ClosedAt,
	)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("task last insert id: %w", err)
	}

	created, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	*t = created

	return nil
}

func (r *taskRepo) GetByID(ctx context.Context, id int64) (domain.Task, error) {
	return r.getOne(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id)
}

func (r *taskRepo) GetByIDForUpdate(ctx context.Context, id int64) (domain.Task, error) {
	return r.getOne(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ? FOR UPDATE`, id)
}

func (r *taskRepo) getOne(ctx context.Context, query string, id int64) (domain.Task, error) {
	t, err := scanTask(r.db.Conn(ctx).QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, fmt.Errorf("task: %w", domain.ErrNotFound)
	}
	if err != nil {
		return t, fmt.Errorf("select task: %w", err)
	}

	return t, nil
}

func (r *taskRepo) Update(ctx context.Context, t *domain.Task, expectedVersion int64) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx, `
		UPDATE tasks
		SET title = ?, description = ?, status = ?, assignee_id = ?, closed_at = ?,
		    updated_at = CURRENT_TIMESTAMP(3), version = version + 1
		WHERE id = ? AND version = ?`,
		t.Title, t.Description, t.Status, t.AssigneeID, t.ClosedAt, t.ID, expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update task rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("task %d was modified concurrently: %w", t.ID, domain.ErrConflict)
	}

	updated, err := r.GetByID(ctx, t.ID)
	if err != nil {
		return err
	}
	*t = updated

	return nil
}

func (r *taskRepo) List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, int64, error) {
	where := []string{"team_id = ?"}
	args := []any{f.TeamID}

	if f.Status != nil {
		where = append(where, "status = ?")
		args = append(args, *f.Status)
	}
	if f.AssigneeID != nil {
		where = append(where, "assignee_id = ?")
		args = append(args, *f.AssigneeID)
	}

	cond := strings.Join(where, " AND ")

	var total int64
	query := `SELECT COUNT(*) FROM tasks WHERE ` + cond
	if err := r.db.Conn(ctx).QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	rows, err := r.db.Conn(ctx).QueryContext(ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE `+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, f.Limit, f.Offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("select tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]domain.Task, 0, f.Limit)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}

	return tasks, total, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(s scanner) (domain.Task, error) {
	var (
		t        domain.Task
		assignee sql.NullInt64
		closedAt sql.NullTime
	)

	err := s.Scan(&t.ID, &t.TeamID, &t.Title, &t.Description, &t.Status, &t.CreatedBy, &assignee,
		&t.CreatedAt, &t.UpdatedAt, &closedAt, &t.Version)
	if err != nil {
		return t, err
	}

	if assignee.Valid {
		t.AssigneeID = &assignee.Int64
	}
	if closedAt.Valid {
		t.ClosedAt = &closedAt.Time
	}

	return t, nil
}
