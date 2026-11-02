package service

import (
	"backend/internal/domain"
	"context"
	"errors"
	"fmt"
)

func (s *taskService) visibleTask(ctx context.Context, userID, taskID int64) (domain.Task, error) {
	task, _, err := s.taskRole(ctx, userID, taskID)
	return task, err
}

// taskRole возвращает задачу и роль пользователя в её команде.
// Для задач чужих команд возвращается ErrNotFound.
func (s *taskService) taskRole(ctx context.Context, userID, taskID int64) (domain.Task, domain.Role, error) {
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return domain.Task{}, "", err
	}

	role, err := s.teamRepo.GetMemberRole(ctx, task.TeamID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Task{}, "", fmt.Errorf("task: %w", domain.ErrNotFound)
		}
		return domain.Task{}, "", err
	}

	return task, role, nil
}

// checkAssignee проверяет, что исполнитель состоит в команде задачи.
func (s *taskService) checkAssignee(ctx context.Context, teamID int64, assigneeID *int64) error {
	if assigneeID == nil {
		return nil
	}

	_, err := s.teamRepo.GetMemberRole(ctx, teamID, *assigneeID)
	if errors.Is(err, domain.ErrNotFound) {
		return validationError("assignee must be a member of the team")
	}

	return err
}

// applyPatch применяет изменения к задаче и возвращает только реально изменившиеся поля.
