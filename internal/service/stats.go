package service

import (
	"context"
	"fmt"

	"backend/internal/domain"
)

// StatsRepository — интерфейс репозитория аналитики.
type StatsRepository interface {
	TeamStats(ctx context.Context, teamID int64) (domain.TeamStats, error)
}

// StatsService — интерфейс сервиса аналитики.
type StatsService interface {
	TeamStats(ctx context.Context, userID, teamID int64) (domain.TeamStats, error)
}

type statsService struct {
	teamRepo  MemberRoleGetter
	statsRepo StatsRepository
}

func NewStatsService(teamRepo MemberRoleGetter, statsRepo StatsRepository) StatsService {
	return &statsService{teamRepo: teamRepo, statsRepo: statsRepo}
}

func (s *statsService) TeamStats(ctx context.Context, userID, teamID int64) (domain.TeamStats, error) {
	role, err := memberRole(ctx, s.teamRepo, teamID, userID)
	if err != nil {
		return domain.TeamStats{}, err
	}

	if !domain.CanViewStats(role) {
		return domain.TeamStats{}, fmt.Errorf("stats are available to owner or admin: %w", domain.ErrForbidden)
	}

	return s.statsRepo.TeamStats(ctx, teamID)
}
