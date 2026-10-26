package api

import (
	"fmt"
	"net/http"
	"strconv"

	"backend/internal/domain"
	"backend/internal/service"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeServiceError(w, r, err)
		return
	}

	user, err := h.authService.Register(r.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  domain.User `json:"user"`
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeServiceError(w, r, err)
		return
	}

	token, user, err := h.authService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: user})
}

type createTeamRequest struct {
	Name string `json:"name"`
}

func (h *handler) createTeam(w http.ResponseWriter, r *http.Request) {
	var req createTeamRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeServiceError(w, r, err)
		return
	}

	team, err := h.teamService.Create(r.Context(), userIDFromContext(r.Context()), req.Name)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, team)
}

func (h *handler) listTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.teamService.List(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": teams})
}

type inviteRequest struct {
	Email string      `json:"email"`
	Role  domain.Role `json:"role"`
}

func (h *handler) inviteMember(w http.ResponseWriter, r *http.Request) {
	teamID, err := pathID(r, "id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	var req inviteRequest
	if err = decodeJSON(w, r, &req); err != nil {
		writeServiceError(w, r, err)
		return
	}

	member, err := h.teamService.Invite(r.Context(), userIDFromContext(r.Context()), teamID, req.Email, req.Role)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, member)
}

type changeRoleRequest struct {
	Role domain.Role `json:"role"`
}

func (h *handler) changeMemberRole(w http.ResponseWriter, r *http.Request) {
	teamID, err := pathID(r, "id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	userID, err := pathID(r, "user_id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	var req changeRoleRequest
	if err = decodeJSON(w, r, &req); err != nil {
		writeServiceError(w, r, err)
		return
	}

	member, err := h.teamService.ChangeRole(r.Context(), userIDFromContext(r.Context()), teamID, userID, req.Role)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, member)
}

func (h *handler) teamStats(w http.ResponseWriter, r *http.Request) {
	teamID, err := pathID(r, "id")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	stats, err := h.statsService.TeamStats(r.Context(), userIDFromContext(r.Context()), teamID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

type createTaskRequest struct {
	TeamID      int64             `json:"team_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      domain.TaskStatus `json:"status"`
	AssigneeID  *int64            `json:"assignee_id"`
}

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
	Version    int64  `json:"version"`
}

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

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type addCommentRequest struct {
	Content string `json:"content"`
}

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

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func queryInt(v, name string) (int64, error) {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, domain.ErrValidation)
	}

	return n, nil
}
