package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"backend/internal/domain"
)

// TeamRepo — интерфейс репозитория команд и участников.
type TeamRepo interface {
	Create(ctx context.Context, t *domain.Team) error
	AddMember(ctx context.Context, m domain.TeamMember) error
	// GetMemberRole возвращает роль пользователя в команде или ErrNotFound, если он не участник.
	GetMemberRole(ctx context.Context, teamID, userID int64) (domain.Role, error)
	UpdateMemberRole(ctx context.Context, m domain.TeamMember) error
	ListByUser(ctx context.Context, userID int64) ([]domain.Team, error)
}

type teamRepo struct {
	db DB
}

func NewTeamRepo(db DB) TeamRepo {
	return &teamRepo{db: db}
}

func (r *teamRepo) Create(ctx context.Context, t *domain.Team) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO teams (name, created_by) VALUES (?, ?)`, t.Name, t.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert team: %w", err)
	}

	if t.ID, err = res.LastInsertId(); err != nil {
		return fmt.Errorf("team last insert id: %w", err)
	}

	err = r.db.Conn(ctx).QueryRowContext(ctx, `SELECT created_at FROM teams WHERE id = ?`, t.ID).Scan(&t.CreatedAt)
	if err != nil {
		return fmt.Errorf("select team: %w", err)
	}

	return nil
}

func (r *teamRepo) AddMember(ctx context.Context, m domain.TeamMember) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role) VALUES (?, ?, ?)`, m.TeamID, m.UserID, m.Role)
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("team member: %w", domain.ErrAlreadyExists)
		}
		return fmt.Errorf("insert team member: %w", err)
	}

	return nil
}

func (r *teamRepo) GetMemberRole(ctx context.Context, teamID, userID int64) (domain.Role, error) {
	var role domain.Role

	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT role FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("team member: %w", domain.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("select team member: %w", err)
	}

	return role, nil
}

func (r *teamRepo) UpdateMemberRole(ctx context.Context, m domain.TeamMember) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ?`, m.Role, m.TeamID, m.UserID)
	if err != nil {
		return fmt.Errorf("update team member: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update team member rows affected: %w", err)
	}
	if n == 0 {
		// Роль могла не измениться — проверяем, что участник существует.
		if _, err = r.GetMemberRole(ctx, m.TeamID, m.UserID); err != nil {
			return err
		}
	}

	return nil
}

func (r *teamRepo) ListByUser(ctx context.Context, userID int64) ([]domain.Team, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, `
		SELECT t.id, t.name, t.created_by, t.created_at, tm.role
		FROM teams t
		JOIN team_members tm ON tm.team_id = t.id
		WHERE tm.user_id = ?
		ORDER BY t.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("select teams: %w", err)
	}
	defer rows.Close()

	teams := make([]domain.Team, 0)
	for rows.Next() {
		var t domain.Team
		if err = rows.Scan(&t.ID, &t.Name, &t.CreatedBy, &t.CreatedAt, &t.Role); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		teams = append(teams, t)
	}

	return teams, rows.Err()
}
