package service

import (
	"context"
	"errors"
	"fmt"

	"backend/internal/domain"
)

// TxManager — интерфейс для выполнения функции в транзакции.
type TxManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// MemberRoleGetter — получение роли пользователя в команде.
type MemberRoleGetter interface {
	GetMemberRole(ctx context.Context, teamID, userID int64) (domain.Role, error)
}

// memberRole возвращает роль пользователя в команде.
// Если пользователь не участник — ErrForbidden: чужие команды не раскрываем.
func memberRole(ctx context.Context, teams MemberRoleGetter, teamID, userID int64) (domain.Role, error) {
	role, err := teams.GetMemberRole(ctx, teamID, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", fmt.Errorf("user is not a member of team %d: %w", teamID, domain.ErrForbidden)
	}

	return role, err
}

func validationError(msg string) error {
	return fmt.Errorf("%s: %w", msg, domain.ErrValidation)
}
