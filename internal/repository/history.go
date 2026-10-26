package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"backend/internal/domain"
)

// HistoryRepo — интерфейс репозитория истории изменений задач.
type HistoryRepo interface {
	Add(ctx context.Context, h *domain.TaskHistory) error
	ListByTask(ctx context.Context, taskID int64) ([]domain.TaskHistory, error)
}

type historyRepo struct {
	db DB
}

func NewHistoryRepo(db DB) HistoryRepo {
	return &historyRepo{db: db}
}

func (r *historyRepo) Add(ctx context.Context, h *domain.TaskHistory) error {
	changes, err := json.Marshal(h.Changes)
	if err != nil {
		return fmt.Errorf("marshal task changes: %w", err)
	}

	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO task_history (task_id, changed_by, changes) VALUES (?, ?, ?)`,
		h.TaskID, h.ChangedBy, changes,
	)
	if err != nil {
		return fmt.Errorf("insert task history: %w", err)
	}

	if h.ID, err = res.LastInsertId(); err != nil {
		return fmt.Errorf("task history last insert id: %w", err)
	}

	return nil
}

func (r *historyRepo) ListByTask(ctx context.Context, taskID int64) ([]domain.TaskHistory, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, `
		SELECT id, task_id, changed_by, changes, created_at
		FROM task_history
		WHERE task_id = ?
		ORDER BY id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("select task history: %w", err)
	}
	defer rows.Close()

	items := make([]domain.TaskHistory, 0)
	for rows.Next() {
		var (
			h   domain.TaskHistory
			raw []byte
		)
		if err = rows.Scan(&h.ID, &h.TaskID, &h.ChangedBy, &raw, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task history: %w", err)
		}
		if err = json.Unmarshal(raw, &h.Changes); err != nil {
			return nil, fmt.Errorf("unmarshal task changes: %w", err)
		}
		items = append(items, h)
	}

	return items, rows.Err()
}
