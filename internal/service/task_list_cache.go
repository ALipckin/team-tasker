package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"backend/internal/cache"
	"backend/internal/domain"
)

type taskListCache struct {
	cache      TaskCache
	ttl        time.Duration
	staleTeams sync.Map
	staleGen   atomic.Uint64
}

func newTaskListCache(taskCache TaskCache, ttl time.Duration) *taskListCache {
	return &taskListCache{cache: taskCache, ttl: ttl}
}

func (c *taskListCache) Get(ctx context.Context, filter domain.TaskFilter) (domain.TaskList, bool) {
	key, cacheable := c.key(ctx, filter)
	if !cacheable {
		return domain.TaskList{}, false
	}

	raw, err := c.cache.Get(ctx, key)
	if err != nil {
		return domain.TaskList{}, false
	}

	var list domain.TaskList
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return domain.TaskList{}, false
	}
	return list, true
}

func (c *taskListCache) Set(ctx context.Context, filter domain.TaskFilter, list domain.TaskList) {
	key, cacheable := c.key(ctx, filter)
	if !cacheable {
		return
	}

	raw, err := json.Marshal(list)
	if err != nil {
		return
	}
	if err := c.cache.Set(ctx, key, string(raw), c.ttl); err != nil {
		slog.WarnContext(ctx, "cache set failed", "key", key, "err", err)
	}
}

func (c *taskListCache) key(ctx context.Context, filter domain.TaskFilter) (string, bool) {
	if _, stale := c.staleTeams.Load(filter.TeamID); stale && !c.Invalidate(ctx, filter.TeamID) {
		return "", false
	}

	version := int64(0)
	raw, err := c.cache.Get(ctx, teamVersionKey(filter.TeamID))
	switch {
	case err == nil:
		if _, scanErr := fmt.Sscan(raw, &version); scanErr != nil {
			return "", false
		}
	case errors.Is(err, cache.ErrMiss):
	default:
		return "", false
	}

	status := ""
	if filter.Status != nil {
		status = string(*filter.Status)
	}
	assignee := ""
	if filter.AssigneeID != nil {
		assignee = fmt.Sprint(*filter.AssigneeID)
	}

	return fmt.Sprintf("tasks:team:%d:v%d:status=%s:assignee=%s:limit=%d:offset=%d",
		filter.TeamID, version, status, assignee, filter.Limit, filter.Offset), true
}

func (c *taskListCache) Invalidate(ctx context.Context, teamID int64) bool {
	ctx = context.WithoutCancel(ctx)

	mark, stale := c.staleTeams.Load(teamID)
	if _, err := c.cache.Incr(ctx, teamVersionKey(teamID)); err != nil {
		if _, already := c.staleTeams.Swap(teamID, c.staleGen.Add(1)); !already {
			slog.WarnContext(ctx, "tasks cache invalidation failed", "team_id", teamID, "err", err)
		}
		return false
	}

	if stale {
		c.staleTeams.CompareAndDelete(teamID, mark)
	}
	return true
}

func teamVersionKey(teamID int64) string {
	return fmt.Sprintf("tasks:team:%d:ver", teamID)
}
