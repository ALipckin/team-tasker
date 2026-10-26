package repository

import (
	"context"
	"fmt"

	"backend/internal/domain"
)

// CommentRepo — интерфейс репозитория комментариев к задачам.
type CommentRepo interface {
	Add(ctx context.Context, c *domain.TaskComment) error
	ListByTask(ctx context.Context, taskID int64) ([]domain.TaskComment, error)
}

type commentRepo struct {
	db DB
}

func NewCommentRepo(db DB) CommentRepo {
	return &commentRepo{db: db}
}

func (r *commentRepo) Add(ctx context.Context, c *domain.TaskComment) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO task_comments (task_id, user_id, content) VALUES (?, ?, ?)`,
		c.TaskID, c.UserID, c.Content,
	)
	if err != nil {
		return fmt.Errorf("insert comment: %w", err)
	}

	if c.ID, err = res.LastInsertId(); err != nil {
		return fmt.Errorf("comment last insert id: %w", err)
	}

	err = r.db.Conn(ctx).QueryRowContext(ctx, `SELECT created_at FROM task_comments WHERE id = ?`, c.ID).Scan(&c.CreatedAt)
	if err != nil {
		return fmt.Errorf("select comment: %w", err)
	}

	return nil
}

func (r *commentRepo) ListByTask(ctx context.Context, taskID int64) ([]domain.TaskComment, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, `
		SELECT id, task_id, user_id, content, created_at
		FROM task_comments
		WHERE task_id = ?
		ORDER BY id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("select comments: %w", err)
	}
	defer rows.Close()

	items := make([]domain.TaskComment, 0)
	for rows.Next() {
		var c domain.TaskComment
		if err = rows.Scan(&c.ID, &c.TaskID, &c.UserID, &c.Content, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		items = append(items, c)
	}

	return items, rows.Err()
}
