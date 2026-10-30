package api

import (
	"fmt"
	"net/http"
	"strconv"

	"backend/internal/domain"
	"backend/internal/service"
)

type createTaskRequest struct {
	TeamID      int64             `json:"team_id" validate:"required" example:"1"`
	Title       string            `json:"title" validate:"required" example:"Настроить CI"`
	Description string            `json:"description" example:"GitHub Actions + линтер"`
	Status      domain.TaskStatus `json:"status" default:"todo"`
	AssigneeID  *int64            `json:"assignee_id"`
}

// createTask godoc
//
//	@Summary	Создать задачу в команде
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		createTaskRequest	true	"Задача"
//	@Success	201		{object}	domain.Task
//	@Failure	400		{object}	errorResponse	"Некорректный запрос"
//	@Failure	401		{object}	errorResponse	"Нет или неверный токен"
//	@Failure	403		{object}	errorResponse	"Недостаточно прав"
//	@Router		/api/v1/tasks [post]
func (h *handler) createTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeServiceError(w, r, err)
		return
	}

	if req.TeamID <= 0 {
		writeError(w, http.StatusBadRequest, "team_id is required")
		return
	}

	task, err := h.taskService.Create(r.Context(), userIDFromContext(r.Context()), service.CreateTaskInput{
		TeamID:      req.TeamID,
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		AssigneeID:  req.AssigneeID,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, task)
}

// listTasks godoc
//
//	@Summary	Список задач команды (кешируется в Redis на 5 минут)
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		team_id		query		int		true	"ID команды"
//	@Param		status		query		string	false	"Статус"	Enums(todo, in_progress, done)
//	@Param		assignee_id	query		int		false	"ID исполнителя"
//	@Param		limit		query		int		false	"Размер страницы"	default(20)	minimum(1)	maximum(100)
//	@Param		offset		query		int		false	"Смещение"			default(0)	minimum(0)
//	@Success	200			{object}	domain.TaskList
//	@Failure	400			{object}	errorResponse	"Некорректный запрос"
//	@Failure	401			{object}	errorResponse	"Нет или неверный токен"
//	@Failure	403			{object}	errorResponse	"Недостаточно прав"
//	@Router		/api/v1/tasks [get]
func (h *handler) listTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	teamID, err := queryInt(q.Get("team_id"), "team_id")
	if err != nil || teamID <= 0 {
		writeError(w, http.StatusBadRequest, "team_id is required and must be a positive integer")
		return
	}

	filter := domain.TaskFilter{TeamID: teamID}

	if v := q.Get("status"); v != "" {
		status := domain.TaskStatus(v)
		filter.Status = &status
	}

	if v := q.Get("assignee_id"); v != "" {
		id, err := queryInt(v, "assignee_id")
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		filter.AssigneeID = &id
	}

	if v := q.Get("limit"); v != "" {
		n, err := queryInt(v, "limit")
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		filter.Limit = int(n)
	}

	if v := q.Get("offset"); v != "" {
		n, err := queryInt(v, "offset")
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		filter.Offset = int(n)
	}

	list, err := h.taskService.List(r.Context(), userIDFromContext(r.Context()), filter)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

// getTask godoc
//
//	@Summary	Получить задачу
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"ID задачи"
//	@Success	200	{object}	domain.Task
//	@Failure	401	{object}	errorResponse	"Нет или неверный токен"
//	@Failure	404	{object}	errorResponse	"Не найдено (в том числе задачи чужих команд)"
//	@Router		/api/v1/tasks/{id} [get]
func (h *handler) getTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := pathID(r, "id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	task, err := h.taskService.Get(r.Context(), userIDFromContext(r.Context()), taskID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, task)
}

type updateTaskRequest struct {
	Title       *string            `json:"title"`
	Description *string            `json:"description"`
	Status      *domain.TaskStatus `json:"status"`
	// AssigneeID: 0 снимает исполнителя.
	AssigneeID *int64 `json:"assignee_id"`
	// Version — текущая версия задачи (optimistic locking).
	Version int64 `json:"version" validate:"required"`
}

// updateTask godoc
//
//	@Summary		Обновить задачу
//	@Description	Передаются только изменяемые поля и обязательная `version` — текущая версия задачи,
//	@Description	полученная клиентом. Если задачу уже изменили, возвращается 409 и нужно перечитать задачу.
//	@Description	Изменение записывается в историю в той же транзакции.
//	@Description
//	@Description	- owner/admin — любые поля;
//	@Description	- создатель задачи — title, description, status, assignee_id;
//	@Description	- исполнитель — только status.
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"ID задачи"
//	@Param			body	body		updateTaskRequest	true	"Изменения"
//	@Success		200		{object}	domain.Task
//	@Failure		400		{object}	errorResponse	"Некорректный запрос"
//	@Failure		401		{object}	errorResponse	"Нет или неверный токен"
//	@Failure		403		{object}	errorResponse	"Недостаточно прав"
//	@Failure		404		{object}	errorResponse	"Не найдено"
//	@Failure		409		{object}	errorResponse	"Задача изменена конкурентно"
//	@Router			/api/v1/tasks/{id} [put]
func (h *handler) updateTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := pathID(r, "id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	var req updateTaskRequest
	if err = decodeJSON(w, r, &req); err != nil {
		writeServiceError(w, r, err)
		return
	}

	task, err := h.taskService.Update(r.Context(), userIDFromContext(r.Context()), taskID, service.UpdateTaskInput{
		TaskPatch: domain.TaskPatch{
			Title:       req.Title,
			Description: req.Description,
			Status:      req.Status,
			AssigneeID:  req.AssigneeID,
		},
		Version: req.Version,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, task)
}

type taskHistoryResponse struct {
	Items []domain.TaskHistory `json:"items"`
}

// taskHistory godoc
//
//	@Summary	История изменений задачи
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"ID задачи"
//	@Success	200	{object}	taskHistoryResponse
//	@Failure	401	{object}	errorResponse	"Нет или неверный токен"
//	@Failure	404	{object}	errorResponse	"Не найдено"
//	@Router		/api/v1/tasks/{id}/history [get]
func (h *handler) taskHistory(w http.ResponseWriter, r *http.Request) {
	taskID, err := pathID(r, "id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	items, err := h.taskService.History(r.Context(), userIDFromContext(r.Context()), taskID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, taskHistoryResponse{Items: items})
}

type addCommentRequest struct {
	Content string `json:"content" validate:"required" example:"Взял в работу"`
}

// addComment godoc
//
//	@Summary	Добавить комментарий
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		int					true	"ID задачи"
//	@Param		body	body		addCommentRequest	true	"Комментарий"
//	@Success	201		{object}	domain.TaskComment
//	@Failure	400		{object}	errorResponse	"Некорректный запрос"
//	@Failure	401		{object}	errorResponse	"Нет или неверный токен"
//	@Failure	404		{object}	errorResponse	"Не найдено"
//	@Router		/api/v1/tasks/{id}/comments [post]
func (h *handler) addComment(w http.ResponseWriter, r *http.Request) {
	taskID, err := pathID(r, "id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	var req addCommentRequest
	if err = decodeJSON(w, r, &req); err != nil {
		writeServiceError(w, r, err)
		return
	}

	comment, err := h.taskService.AddComment(r.Context(), userIDFromContext(r.Context()), taskID, req.Content)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

type commentListResponse struct {
	Items []domain.TaskComment `json:"items"`
}

// listComments godoc
//
//	@Summary	Комментарии задачи
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"ID задачи"
//	@Success	200	{object}	commentListResponse
//	@Failure	401	{object}	errorResponse	"Нет или неверный токен"
//	@Failure	404	{object}	errorResponse	"Не найдено"
//	@Router		/api/v1/tasks/{id}/comments [get]
func (h *handler) listComments(w http.ResponseWriter, r *http.Request) {
	taskID, err := pathID(r, "id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	items, err := h.taskService.Comments(r.Context(), userIDFromContext(r.Context()), taskID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, commentListResponse{Items: items})
}

func queryInt(v, name string) (int64, error) {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, domain.ErrValidation)
	}

	return n, nil
}
