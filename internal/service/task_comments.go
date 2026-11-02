package service

import (
	"backend/internal/domain"
	"context"
	"strings"
)

func (s *taskService) History(ctx context.Context, userID, taskID int64) ([]domain.TaskHistory, error) {
	if _, err := s.visibleTask(ctx, userID, taskID); err != nil {
		return nil, err
	}

	return s.historyRepo.ListByTask(ctx, taskID)
}

func (s *taskService) AddComment(
	ctx context.Context, userID, taskID int64, content string,
) (domain.TaskComment, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return domain.TaskComment{}, validationError("content is required")
	}
	if len(content) > maxContentLen {
		return domain.TaskComment{}, validationError("content is too long")
	}

	if _, err := s.visibleTask(ctx, userID, taskID); err != nil {
		return domain.TaskComment{}, err
	}

	c := domain.TaskComment{TaskID: taskID, UserID: userID, Content: content}
	if err := s.commentRepo.Add(ctx, &c); err != nil {
		return domain.TaskComment{}, err
	}

	return c, nil
}

func (s *taskService) Comments(ctx context.Context, userID, taskID int64) ([]domain.TaskComment, error) {
	if _, err := s.visibleTask(ctx, userID, taskID); err != nil {
		return nil, err
	}

	return s.commentRepo.ListByTask(ctx, taskID)
}

// visibleTask возвращает задачу, если пользователь состоит в её команде.
// Для задач чужих команд возвращается ErrNotFound.
