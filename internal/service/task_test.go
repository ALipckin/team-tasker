package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"backend/internal/cache"
	"backend/internal/domain"
)

type fakeTx struct{}

func (fakeTx) WithTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

type fakeTeams map[[2]int64]domain.Role

func (f fakeTeams) GetMemberRole(_ context.Context, teamID, userID int64) (domain.Role, error) {
	role, ok := f[[2]int64{teamID, userID}]
	if !ok {
		return "", domain.ErrNotFound
	}
	return role, nil
}

type fakeTasks struct {
	tasks  map[int64]domain.Task
	nextID int64
}

func (f *fakeTasks) Create(_ context.Context, t *domain.Task) error {
	f.nextID++
	t.ID, t.Version = f.nextID, 1
	f.tasks[t.ID] = *t
	return nil
}

func (f *fakeTasks) GetByID(_ context.Context, id int64) (domain.Task, error) {
	t, ok := f.tasks[id]
	if !ok {
		return t, domain.ErrNotFound
	}
	return t, nil
}

func (f *fakeTasks) GetByIDForUpdate(ctx context.Context, id int64) (domain.Task, error) {
	return f.GetByID(ctx, id)
}

func (f *fakeTasks) Update(_ context.Context, t *domain.Task, expected int64) error {
	if f.tasks[t.ID].Version != expected {
		return domain.ErrConflict
	}
	t.Version = expected + 1
	f.tasks[t.ID] = *t
	return nil
}

func (f *fakeTasks) List(context.Context, domain.TaskFilter) ([]domain.Task, int64, error) {
	items := make([]domain.Task, 0, len(f.tasks))
	for _, t := range f.tasks {
		items = append(items, t)
	}
	return items, int64(len(items)), nil
}

type fakeHistory struct{ items []domain.TaskHistory }

func (f *fakeHistory) Add(_ context.Context, h *domain.TaskHistory) error {
	f.items = append(f.items, *h)
	return nil
}

func (f *fakeHistory) ListByTask(context.Context, int64) ([]domain.TaskHistory, error) {
	return f.items, nil
}

type fakeComments struct{}

func (fakeComments) Add(context.Context, *domain.TaskComment) error { return nil }
func (fakeComments) ListByTask(context.Context, int64) ([]domain.TaskComment, error) {
	return nil, nil
}

type fakeCache struct {
	data     map[string]string
	reads    int
	incrErr  error
	incrCtxs []context.Context
	// afterIncr вызывается после успешного Incr, до возврата.
	afterIncr func()
}

func (f *fakeCache) Get(_ context.Context, key string) (string, error) {
	f.reads++
	v, ok := f.data[key]
	if !ok {
		return "", cache.ErrMiss
	}
	return v, nil
}

func (f *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.data[key] = value
	return nil
}

func (f *fakeCache) Incr(ctx context.Context, key string) (int64, error) {
	f.incrCtxs = append(f.incrCtxs, ctx)
	if f.incrErr != nil {
		return 0, f.incrErr
	}
	var n int64
	if raw, ok := f.data[key]; ok {
		if _, err := fmt.Sscan(raw, &n); err != nil {
			return 0, fmt.Errorf("parse cache counter: %w", err)
		}
	}
	n++
	f.data[key] = fmt.Sprint(n)
	if f.afterIncr != nil {
		f.afterIncr()
	}
	return n, nil
}

const (
	teamID   int64 = 1
	owner    int64 = 10
	creator  int64 = 11
	assignee int64 = 12
	member   int64 = 13
	outsider int64 = 99
)

type fixture struct {
	svc     *taskService
	tasks   *fakeTasks
	history *fakeHistory
	cache   *fakeCache
	task    domain.Task
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	teams := fakeTeams{
		{teamID, owner}:    domain.RoleOwner,
		{teamID, creator}:  domain.RoleMember,
		{teamID, assignee}: domain.RoleMember,
		{teamID, member}:   domain.RoleMember,
		{2, outsider}:      domain.RoleOwner,
	}

	f := &fixture{
		tasks:   &fakeTasks{tasks: map[int64]domain.Task{}},
		history: &fakeHistory{},
		cache:   &fakeCache{data: map[string]string{}},
	}
	svc, ok := NewTaskService(fakeTx{}, f.tasks, teams, f.history, fakeComments{}, f.cache, time.Minute).(*taskService)
	if !ok {
		t.Fatal("NewTaskService returned unexpected implementation")
	}
	f.svc = svc

	a := assignee
	task, err := f.svc.Create(context.Background(), creator, CreateTaskInput{
		TeamID: teamID, Title: "task", AssigneeID: &a,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	f.task = task

	return f
}

func ptr[T any](v T) *T { return &v }

func TestTaskUpdatePermissions(t *testing.T) {
	tests := []struct {
		name    string
		userID  int64
		patch   domain.TaskPatch
		wantErr error
	}{
		{"owner changes title", owner, domain.TaskPatch{Title: ptr("new")}, nil},
		{"creator reassigns to member", creator, domain.TaskPatch{AssigneeID: ptr(member)}, nil},
		{"creator cannot assign outsider", creator, domain.TaskPatch{AssigneeID: ptr(outsider)}, domain.ErrValidation},
		{"assignee changes status", assignee, domain.TaskPatch{Status: ptr(domain.StatusInProgress)}, nil},
		{"assignee cannot reassign", assignee, domain.TaskPatch{AssigneeID: ptr(member)}, domain.ErrForbidden},
		{"assignee cannot change title", assignee, domain.TaskPatch{Title: ptr("x")}, domain.ErrForbidden},
		{"other member cannot edit", member, domain.TaskPatch{Status: ptr(domain.StatusDone)}, domain.ErrForbidden},
		{"outsider does not see task", outsider, domain.TaskPatch{Status: ptr(domain.StatusDone)}, domain.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)

			_, err := f.svc.Update(context.Background(), tt.userID, f.task.ID, UpdateTaskInput{TaskPatch: tt.patch, Version: 1})
			if tt.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestTaskUpdateWritesHistoryAndInvalidatesCache(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.svc.List(ctx, member, domain.TaskFilter{TeamID: teamID}); err != nil {
		t.Fatalf("list: %v", err)
	}
	verBefore := f.cache.data[teamVersionKey(teamID)]

	updated, err := f.svc.Update(ctx, assignee, f.task.ID, UpdateTaskInput{
		TaskPatch: domain.TaskPatch{Status: ptr(domain.StatusDone)},
		Version:   1,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if updated.Version != 2 || updated.ClosedAt == nil || updated.Status != domain.StatusDone {
		t.Fatalf("unexpected task after update: %+v", updated)
	}

	last := f.history.items[len(f.history.items)-1]
	if last.ChangedBy != assignee {
		t.Errorf("history author = %d, want %d", last.ChangedBy, assignee)
	}
	if ch := last.Changes["status"]; ch.Old != domain.StatusTodo || ch.New != domain.StatusDone {
		t.Errorf("status change = %+v", ch)
	}
	if _, ok := last.Changes["closed_at"]; !ok {
		t.Error("closed_at change is not recorded")
	}

	if f.cache.data[teamVersionKey(teamID)] == verBefore {
		t.Error("cache version was not bumped")
	}

	// Повторное обновление со старой версией — конфликт, а не потерянное изменение.
	_, err = f.svc.Update(ctx, owner, f.task.ID, UpdateTaskInput{
		TaskPatch: domain.TaskPatch{Title: ptr("stale")}, Version: 1,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestTaskUpdateRecordsOnlyRealChanges(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.Update(context.Background(), owner, f.task.ID, UpdateTaskInput{
		TaskPatch: domain.TaskPatch{Status: ptr(domain.StatusInProgress), Title: ptr("task")},
		Version:   1,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	last := f.history.items[len(f.history.items)-1]
	if len(last.Changes) != 1 {
		t.Fatalf("expected only status change, got %+v", last.Changes)
	}
}

func TestTaskListRequiresMembership(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.List(context.Background(), outsider, domain.TaskFilter{TeamID: teamID})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if f.cache.reads != 0 {
		t.Error("cache must not be read before access check")
	}
}

func TestTaskListUsesCache(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	first, err := f.svc.List(ctx, member, domain.TaskFilter{TeamID: teamID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	// Меняем данные в обход сервиса: ответ должен прийти из кэша.
	f.tasks.tasks[999] = domain.Task{ID: 999, TeamID: teamID}

	second, err := f.svc.List(ctx, member, domain.TaskFilter{TeamID: teamID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if second.Total != first.Total {
		t.Errorf("expected cached result with total %d, got %d", first.Total, second.Total)
	}
}

func TestTaskNoopUpdateKeepsCache(t *testing.T) {
	f := newFixture(t)
	verBefore := f.cache.data[teamVersionKey(teamID)]

	_, err := f.svc.Update(context.Background(), owner, f.task.ID, UpdateTaskInput{
		TaskPatch: domain.TaskPatch{Title: ptr("task")},
		Version:   1,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if f.cache.data[teamVersionKey(teamID)] != verBefore {
		t.Error("no-op update must not invalidate cache")
	}
}

func TestTaskInvalidationSurvivesCanceledRequest(t *testing.T) {
	f := newFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f.svc.listCache.Invalidate(ctx, teamID)

	last := f.cache.incrCtxs[len(f.cache.incrCtxs)-1]
	if last.Err() != nil {
		t.Fatalf("invalidation must not use a canceled context: %v", last.Err())
	}
}

func TestTaskListBypassesCacheUntilInvalidationSucceeds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.svc.List(ctx, member, domain.TaskFilter{TeamID: teamID}); err != nil {
		t.Fatalf("list: %v", err)
	}

	// Redis недоступен в момент изменения: версия команды не увеличилась.
	f.cache.incrErr = errors.New("redis down")
	f.tasks.tasks[999] = domain.Task{ID: 999, TeamID: teamID}
	f.svc.listCache.Invalidate(ctx, teamID)

	list, err := f.svc.List(ctx, member, domain.TaskFilter{TeamID: teamID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("stale cache served while invalidation is pending: total %d", list.Total)
	}

	// Redis восстановился: инвалидация повторяется, кэш снова используется.
	f.cache.incrErr = nil
	if _, err = f.svc.List(ctx, member, domain.TaskFilter{TeamID: teamID}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, stale := f.svc.listCache.staleTeams.Load(teamID); stale {
		t.Error("team must not stay stale after successful invalidation")
	}
}

func TestTaskStaleMarkSurvivesConcurrentFailure(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	f.cache.incrErr = errors.New("redis down")
	f.svc.listCache.Invalidate(ctx, teamID)
	f.cache.incrErr = nil

	// Повторная инвалидация проходит, но сразу после её Incr падает
	// инвалидация параллельного изменения.
	f.cache.afterIncr = func() {
		f.cache.afterIncr = nil
		f.cache.incrErr = errors.New("redis down")
		f.svc.listCache.Invalidate(ctx, teamID)
		f.cache.incrErr = nil
	}
	f.svc.listCache.Invalidate(ctx, teamID)

	if _, stale := f.svc.listCache.staleTeams.Load(teamID); !stale {
		t.Error("failed concurrent invalidation must keep the team stale")
	}
}

func TestTaskTitleLengthCountsCharacters(t *testing.T) {
	f := newFixture(t)

	title := strings.Repeat("ж", maxTitleLen)
	if _, err := f.svc.Create(context.Background(), owner, CreateTaskInput{TeamID: teamID, Title: title}); err != nil {
		t.Fatalf("255 cyrillic characters must fit: %v", err)
	}

	_, err := f.svc.Create(context.Background(), owner, CreateTaskInput{TeamID: teamID, Title: title + "ж"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
