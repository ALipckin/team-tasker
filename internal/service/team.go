package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"backend/internal/domain"
)

// TeamRepository — интерфейс репозитория команд для TeamService.
type TeamRepository interface {
	MemberRoleGetter
	Create(ctx context.Context, t *domain.Team) error
	AddMember(ctx context.Context, m domain.TeamMember) error
	UpdateMemberRole(ctx context.Context, m domain.TeamMember) error
	ListByUser(ctx context.Context, userID int64) ([]domain.Team, error)
}

// TeamUserRepository — интерфейс репозитория пользователей для TeamService.
type TeamUserRepository interface {
	GetByEmail(ctx context.Context, email string) (domain.User, error)
}

// TeamService — интерфейс сервиса команд.
type TeamService interface {
	Create(ctx context.Context, userID int64, name string) (domain.Team, error)
	List(ctx context.Context, userID int64) ([]domain.Team, error)
	Invite(ctx context.Context, actorID, teamID int64, email string, role domain.Role) (domain.TeamMember, error)
	ChangeRole(ctx context.Context, actorID, teamID, userID int64, role domain.Role) (domain.TeamMember, error)
}

type teamService struct {
	tx       TxManager
	teamRepo TeamRepository
	userRepo TeamUserRepository
}

func NewTeamService(tx TxManager, teamRepo TeamRepository, userRepo TeamUserRepository) TeamService {
	return &teamService{tx: tx, teamRepo: teamRepo, userRepo: userRepo}
}

// Create создаёт команду и добавляет создателя участником с ролью owner в одной транзакции.
func (s *teamService) Create(ctx context.Context, userID int64, name string) (domain.Team, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Team{}, validationError("name is required")
	}
	if utf8.RuneCountInString(name) > maxNameLen {
		return domain.Team{}, validationError("name is too long")
	}

	team := domain.Team{Name: name, CreatedBy: userID, Role: domain.RoleOwner}

	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.teamRepo.Create(ctx, &team); err != nil {
			return err
		}

		return s.teamRepo.AddMember(ctx, domain.TeamMember{TeamID: team.ID, UserID: userID, Role: domain.RoleOwner})
	})
	if err != nil {
		return domain.Team{}, err
	}

	slog.InfoContext(ctx, "team created", "team_id", team.ID, "user_id", userID)

	return team, nil
}

func (s *teamService) List(ctx context.Context, userID int64) ([]domain.Team, error) {
	return s.teamRepo.ListByUser(ctx, userID)
}

// Invite добавляет зарегистрированного пользователя в команду.
func (s *teamService) Invite(ctx context.Context, actorID, teamID int64, email string, role domain.Role) (domain.TeamMember, error) {
	if role == "" {
		role = domain.RoleMember
	}
	if role != domain.RoleMember && role != domain.RoleAdmin {
		return domain.TeamMember{}, validationError("role must be admin or member")
	}

	var member domain.TeamMember

	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		actorRole, err := memberRole(ctx, s.teamRepo, teamID, actorID)
		if err != nil {
			return err
		}
		if !domain.CanInvite(actorRole, role) {
			return fmt.Errorf("role %s cannot invite %s: %w", actorRole, role, domain.ErrForbidden)
		}

		user, err := s.userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("user with this email is not registered: %w", domain.ErrNotFound)
		}
		if err != nil {
			return err
		}

		member = domain.TeamMember{TeamID: teamID, UserID: user.ID, Role: role}

		return s.teamRepo.AddMember(ctx, member)
	})
	if err != nil {
		return domain.TeamMember{}, err
	}

	slog.InfoContext(ctx, "team member invited", "team_id", teamID, "user_id", member.UserID, "role", role, "by", actorID)

	return member, nil
}

// ChangeRole меняет роль участника. Доступно только создателю команды.
func (s *teamService) ChangeRole(ctx context.Context, actorID, teamID, userID int64, role domain.Role) (domain.TeamMember, error) {
	if !role.Valid() {
		return domain.TeamMember{}, validationError("invalid role")
	}

	member := domain.TeamMember{TeamID: teamID, UserID: userID, Role: role}

	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		actorRole, err := memberRole(ctx, s.teamRepo, teamID, actorID)
		if err != nil {
			return err
		}

		currentRole, err := s.teamRepo.GetMemberRole(ctx, teamID, userID)
		if err != nil {
			return err
		}

		if !domain.CanChangeRole(actorRole, currentRole, role) {
			return fmt.Errorf("cannot change role %s to %s: %w", currentRole, role, domain.ErrForbidden)
		}

		return s.teamRepo.UpdateMemberRole(ctx, member)
	})
	if err != nil {
		return domain.TeamMember{}, err
	}

	return member, nil
}
