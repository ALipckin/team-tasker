package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"backend/internal/cache"
	"backend/internal/domain"
)

func teamVersionKey(teamID int64) string {
	return fmt.Sprintf("tasks:team:%d:ver", teamID)
}

func (s *taskService) listCacheKey(ctx context.Context, f domain.TaskFilter) (string, bool) {
	if _, stale := s.staleTeams.Load(f.TeamID); stale && !s.invalidateTeam(ctx, f.TeamID) {
		// Инвалидация так и не прошла — кэш команды может быть устаревшим.
		return "", false
	}

	ver := int64(0)

	raw, err := s.cache.Get(ctx, teamVersionKey(f.TeamID))
	switch {
	case err == nil:
		if _, scanErr := fmt.Sscan(raw, &ver); scanErr != nil {
			return "", false
		}
	case isCacheMiss(err):
	default:
		// Кэш недоступен — идём в БД.
		return "", false
	}

	status := ""
	if f.Status != nil {
		status = string(*f.Status)
	}
	assignee := ""
	if f.AssigneeID != nil {
		assignee = fmt.Sprint(*f.AssigneeID)
	}

	return fmt.Sprintf("tasks:team:%d:v%d:status=%s:assignee=%s:limit=%d:offset=%d",
		f.TeamID, ver, status, assignee, f.Limit, f.Offset), true
}

// invalidateTeam сбрасывает кэш списков команды.
// При ошибке команда помечается устаревшей: её списки идут мимо кэша,
// пока очередная попытка инвалидации не пройдёт.
func (s *taskService) invalidateTeam(ctx context.Context, teamID int64) bool {
	ctx = context.WithoutCancel(ctx)

	// Запоминаем отметку до Incr: если параллельная инвалидация упадёт после
	// нашего успешного Incr, она запишет новую отметку, и снимать её нельзя.
	mark, stale := s.staleTeams.Load(teamID)

	if _, err := s.cache.Incr(ctx, teamVersionKey(teamID)); err != nil {
		if _, already := s.staleTeams.Swap(teamID, s.staleGen.Add(1)); !already {
			slog.WarnContext(ctx, "tasks cache invalidation failed", "team_id", teamID, "err", err)
		}
		return false
	}

	if stale {
		s.staleTeams.CompareAndDelete(teamID, mark)
	}
	return true
}

func isCacheMiss(err error) bool {
	return errors.Is(err, cache.ErrMiss)
}
