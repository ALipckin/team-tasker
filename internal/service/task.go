package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"backend/internal/cache"
	"backend/internal/domain"
)

// TaskRepository — интерфейс репозитория задач для TaskService.
type TaskRepository interface {
	Create(ctx context.Context, t *domain.Task) error
	GetByID(ctx context.Context, id int64) (domain.Task, error)
	GetByIDForUpdate(ctx context.Context, id int64) (domain.Task, error)
	Update(ctx context.Context, t *domain.Task, expectedVersion int64) error
	List(ctx context.Context, f domain.TaskFilter) ([]domain.Task, int64, error)
}

// TaskHistoryRepository — интерфейс репозитория истории задач.
type TaskHistoryRepository interface {
	Add(ctx context.Context, h *domain.TaskHistory) error
	ListByTask(ctx context.Context, taskID int64) ([]domain.TaskHistory, error)
}

// TaskCommentRepository — интерфейс репозитория комментариев.
type TaskCommentRepository interface {
	Add(ctx context.Context, c *domain.TaskComment) error
	ListByTask(ctx context.Context, taskID int64) ([]domain.TaskComment, error)
}

// TaskCache — интерфейс кэша для списка задач.
type TaskCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Incr(ctx context.Context, key string) (int64, error)
}

type CreateTaskInput struct {
	TeamID      int64
	Title       string
	Description string
	Status      domain.TaskStatus
	AssigneeID  *int64
}

type UpdateTaskInput struct {
	domain.TaskPatch
	// Version — версия задачи, которую видел клиент. Защищает от потерянных обновлений.
	Version int64
}

// TaskService — интерфейс сервиса задач.
type TaskService interface {
	Create(ctx context.Context, userID int64, in CreateTaskInput) (domain.Task, error)
	Get(ctx context.Context, userID, taskID int64) (domain.Task, error)
	List(ctx context.Context, userID int64, f domain.TaskFilter) (domain.TaskList, error)
	Update(ctx context.Context, userID, taskID int64, in UpdateTaskInput) (domain.Task, error)
	History(ctx context.Context, userID, taskID int64) ([]domain.TaskHistory, error)
	AddComment(ctx context.Context, userID, taskID int64, content string) (domain.TaskComment, error)
	Comments(ctx context.Context, userID, taskID int64) ([]domain.TaskComment, error)
}

type taskService struct {
	tx          TxManager
	taskRepo    TaskRepository
	teamRepo    MemberRoleGetter
	historyRepo TaskHistoryRepository
	commentRepo TaskCommentRepository
	cache       TaskCache
	cacheTTL    time.Duration
	now         func() time.Time

	// staleTeams — команды, для которых не удалось инвалидировать кэш.
	// Пока команда здесь, её списки читаются из БД, а инвалидация повторяется.
	// Значение — номер последней неудачной попытки из staleGen.
	staleTeams sync.Map
	staleGen   atomic.Uint64
}

func NewTaskService(
	tx TxManager,
	taskRepo TaskRepository,
	teamRepo MemberRoleGetter,
	historyRepo TaskHistoryRepository,
	commentRepo TaskCommentRepository,
	cache TaskCache,
	cacheTTL time.Duration,
) TaskService {
	return &taskService{
		tx:          tx,
		taskRepo:    taskRepo,
		teamRepo:    teamRepo,
		historyRepo: historyRepo,
		commentRepo: commentRepo,
		cache:       cache,
		cacheTTL:    cacheTTL,
		now:         time.Now,
	}
}

const (
	maxTitleLen   = 255
	maxContentLen = 65535
	DefaultLimit  = 20
	MaxLimit      = 100
)

func (s *taskService) Create(ctx context.Context, userID int64, in CreateTaskInput) (domain.Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return domain.Task{}, validationError("title is required")
	}
	if utf8.RuneCountInString(in.Title) > maxTitleLen {
		return domain.Task{}, validationError("title is too long")
	}
	if len(in.Description) > maxContentLen {
		return domain.Task{}, validationError("description is too long")
	}
	if in.Status == "" {
		in.Status = domain.StatusTodo
	}
	if !in.Status.Valid() {
		return domain.Task{}, validationError("invalid status")
	}
	if in.AssigneeID != nil && *in.AssigneeID == 0 {
		in.AssigneeID = nil
	}

	task := domain.Task{
		TeamID:      in.TeamID,
		Title:       in.Title,
		Description: in.Description,
		Status:      in.Status,
		CreatedBy:   userID,
		AssigneeID:  in.AssigneeID,
	}
	if task.Status == domain.StatusDone {
		task.ClosedAt = s.nowPtr()
	}

	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := memberRole(ctx, s.teamRepo, in.TeamID, userID); err != nil {
			return err
		}

		if err := s.checkAssignee(ctx, in.TeamID, task.AssigneeID); err != nil {
			return err
		}

		if err := s.taskRepo.Create(ctx, &task); err != nil {
			return err
		}

		return s.historyRepo.Add(ctx, &domain.TaskHistory{
			TaskID:    task.ID,
			ChangedBy: userID,
			Changes: map[string]domain.FieldChange{
				"title":       {Old: nil, New: task.Title},
				"description": {Old: nil, New: task.Description},
				"status":      {Old: nil, New: task.Status},
				"assignee_id": {Old: nil, New: task.AssigneeID},
			},
		})
	})
	if err != nil {
		return domain.Task{}, err
	}

	s.invalidateTeam(ctx, task.TeamID)
	slog.InfoContext(ctx, "task created", "task_id", task.ID, "team_id", task.TeamID, "user_id", userID)

	return task, nil
}

func (s *taskService) Get(ctx context.Context, userID, taskID int64) (domain.Task, error) {
	return s.visibleTask(ctx, userID, taskID)
}

func (s *taskService) List(ctx context.Context, userID int64, f domain.TaskFilter) (domain.TaskList, error) {
	if f.TeamID <= 0 {
		return domain.TaskList{}, validationError("team_id is required")
	}
	if f.Status != nil && !f.Status.Valid() {
		return domain.TaskList{}, validationError("invalid status")
	}
	if f.Limit <= 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit > MaxLimit {
		f.Limit = MaxLimit
	}
	if f.Offset < 0 {
		return domain.TaskList{}, validationError("offset must be non-negative")
	}

	// Проверка доступа выполняется до кэша: кэш общий для всех участников команды.
	if _, err := memberRole(ctx, s.teamRepo, f.TeamID, userID); err != nil {
		return domain.TaskList{}, err
	}

	key, cacheable := s.listCacheKey(ctx, f)
	if cacheable {
		if raw, err := s.cache.Get(ctx, key); err == nil {
			var list domain.TaskList
			if err = json.Unmarshal([]byte(raw), &list); err == nil {
				return list, nil
			}
		}
	}

	items, total, err := s.taskRepo.List(ctx, f)
	if err != nil {
		return domain.TaskList{}, err
	}

	list := domain.TaskList{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}

	if cacheable {
		if raw, err := json.Marshal(list); err == nil {
			if err = s.cache.Set(ctx, key, string(raw), s.cacheTTL); err != nil {
				slog.WarnContext(ctx, "cache set failed", "key", key, "err", err)
			}
		}
	}

	return list, nil
}

// Update изменяет задачу. Внутри транзакции строка блокируется через SELECT ... FOR UPDATE,
// поэтому история строится от актуального состояния, а конкурентные обновления
// выполняются последовательно. Версия из запроса должна совпадать с текущей —
// иначе клиент редактировал устаревшие данные и получает ErrConflict.
func (s *taskService) Update(ctx context.Context, userID, taskID int64, in UpdateTaskInput) (domain.Task, error) {
	if in.Version <= 0 {
		return domain.Task{}, validationError("version is required")
	}
	if in.TaskPatch.Empty() {
		return domain.Task{}, validationError("nothing to update")
	}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return domain.Task{}, validationError("title must not be empty")
		}
		if utf8.RuneCountInString(t) > maxTitleLen {
			return domain.Task{}, validationError("title is too long")
		}
		in.Title = &t
	}
	if in.Description != nil && len(*in.Description) > maxContentLen {
		return domain.Task{}, validationError("description is too long")
	}
	if in.Status != nil && !in.Status.Valid() {
		return domain.Task{}, validationError("invalid status")
	}

	// Доступ проверяем до блокировки, чтобы посторонний не мог блокировать строки чужих задач.
	// Команда задачи не меняется, поэтому роль можно не перечитывать в транзакции.
	_, role, err := s.taskRole(ctx, userID, taskID)
	if err != nil {
		return domain.Task{}, err
	}

	var (
		task    domain.Task
		changed bool
	)

	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		current, err := s.taskRepo.GetByIDForUpdate(ctx, taskID)
		if err != nil {
			return err
		}

		if !domain.CanEditTask(userID, role, current, in.TaskPatch) {
			return fmt.Errorf("user %d cannot edit task %d: %w", userID, taskID, domain.ErrForbidden)
		}

		if current.Version != in.Version {
			return fmt.Errorf("task version is %d, got %d: %w", current.Version, in.Version, domain.ErrConflict)
		}

		task = current
		changes := s.applyPatch(&task, in.TaskPatch)
		if len(changes) == 0 {
			return nil
		}

		if _, ok := changes["assignee_id"]; ok {
			if err = s.checkAssignee(ctx, task.TeamID, task.AssigneeID); err != nil {
				return err
			}
		}

		if err = s.taskRepo.Update(ctx, &task, current.Version); err != nil {
			return err
		}

		changed = true
		return s.historyRepo.Add(ctx, &domain.TaskHistory{TaskID: task.ID, ChangedBy: userID, Changes: changes})
	})
	if err != nil {
		return domain.Task{}, err
	}

	if !changed {
		return task, nil
	}

	s.invalidateTeam(ctx, task.TeamID)
	slog.InfoContext(ctx, "task updated", "task_id", task.ID, "user_id", userID, "version", task.Version)

	return task, nil
}

func (s *taskService) History(ctx context.Context, userID, taskID int64) ([]domain.TaskHistory, error) {
	if _, err := s.visibleTask(ctx, userID, taskID); err != nil {
		return nil, err
	}

	return s.historyRepo.ListByTask(ctx, taskID)
}

func (s *taskService) AddComment(ctx context.Context, userID, taskID int64, content string) (domain.TaskComment, error) {
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
func (s *taskService) applyPatch(t *domain.Task, p domain.TaskPatch) map[string]domain.FieldChange {
	changes := make(map[string]domain.FieldChange)

	if p.Title != nil && *p.Title != t.Title {
		changes["title"] = domain.FieldChange{Old: t.Title, New: *p.Title}
		t.Title = *p.Title
	}

	if p.Description != nil && *p.Description != t.Description {
		changes["description"] = domain.FieldChange{Old: t.Description, New: *p.Description}
		t.Description = *p.Description
	}

	if p.Status != nil && *p.Status != t.Status {
		changes["status"] = domain.FieldChange{Old: t.Status, New: *p.Status}

		oldClosed := t.ClosedAt
		switch {
		case *p.Status == domain.StatusDone:
			t.ClosedAt = s.nowPtr()
		case t.Status == domain.StatusDone:
			t.ClosedAt = nil
		}
		if oldClosed != nil || t.ClosedAt != nil {
			changes["closed_at"] = domain.FieldChange{Old: oldClosed, New: t.ClosedAt}
		}

		t.Status = *p.Status
	}

	if p.AssigneeID != nil {
		var newAssignee *int64
		if *p.AssigneeID != 0 {
			v := *p.AssigneeID
			newAssignee = &v
		}

		if !equalIDs(t.AssigneeID, newAssignee) {
			changes["assignee_id"] = domain.FieldChange{Old: t.AssigneeID, New: newAssignee}
			t.AssigneeID = newAssignee
		}
	}

	return changes
}

func (s *taskService) nowPtr() *time.Time {
	// В MySQL храним миллисекунды — обрезаем заранее, чтобы история совпадала с БД.
	now := s.now().UTC().Truncate(time.Millisecond)
	return &now
}

func equalIDs(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Кэш списка задач версионируется по команде: ключ включает номер версии команды,
// а любое изменение задач увеличивает его. Старые ключи становятся недостижимыми
// и истекают по TTL. Так инвалидируются сразу все комбинации фильтров команды.
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

// invalidateTeam сбрасывает кэш списков команды. Изменение уже закоммичено,
// поэтому отмена запроса клиентом не должна прерывать инвалидацию.
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
