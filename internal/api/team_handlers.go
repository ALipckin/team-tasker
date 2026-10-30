package api

import (
	"net/http"

	"backend/internal/domain"
)

type createTeamRequest struct {
	Name string `json:"name" validate:"required" example:"Backend"`
}

// createTeam godoc
//
//	@Summary	Создание команды (текущий пользователь становится owner)
//	@Tags		teams
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		createTeamRequest	true	"Команда"
//	@Success	201		{object}	domain.Team
//	@Failure	400		{object}	errorResponse	"Некорректный запрос"
//	@Failure	401		{object}	errorResponse	"Нет или неверный токен"
//	@Router		/api/v1/teams [post]
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

type teamListResponse struct {
	Items []domain.Team `json:"items"`
}

// listTeams godoc
//
//	@Summary	Команды текущего пользователя
//	@Tags		teams
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	teamListResponse
//	@Failure	401	{object}	errorResponse	"Нет или неверный токен"
//	@Router		/api/v1/teams [get]
func (h *handler) listTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.teamService.List(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, teamListResponse{Items: teams})
}

type inviteRequest struct {
	Email string      `json:"email" validate:"required" format:"email" example:"bob@example.com"`
	Role  domain.Role `json:"role" enums:"admin,member" default:"member"`
}

// inviteMember godoc
//
//	@Summary		Добавить пользователя в команду (owner или admin)
//	@Description	Пользователь должен быть зарегистрирован. Роль `owner` выдать нельзя.
//	@Description	Роль `admin` может выдать только owner.
//	@Tags			teams
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"ID команды"
//	@Param			body	body		inviteRequest	true	"Приглашение"
//	@Success		201		{object}	domain.TeamMember
//	@Failure		400		{object}	errorResponse	"Некорректный запрос"
//	@Failure		401		{object}	errorResponse	"Нет или неверный токен"
//	@Failure		403		{object}	errorResponse	"Недостаточно прав"
//	@Failure		404		{object}	errorResponse	"Не найдено"
//	@Failure		409		{object}	errorResponse	"Уже участник команды"
//	@Router			/api/v1/teams/{id}/invite [post]
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
	Role domain.Role `json:"role" validate:"required" enums:"admin,member"`
}

// changeMemberRole godoc
//
//	@Summary	Изменить роль участника (только owner)
//	@Tags		teams
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		int					true	"ID команды"
//	@Param		user_id	path		int					true	"ID пользователя"
//	@Param		body	body		changeRoleRequest	true	"Новая роль"
//	@Success	200		{object}	domain.TeamMember
//	@Failure	400		{object}	errorResponse	"Некорректный запрос"
//	@Failure	401		{object}	errorResponse	"Нет или неверный токен"
//	@Failure	403		{object}	errorResponse	"Недостаточно прав"
//	@Failure	404		{object}	errorResponse	"Не найдено"
//	@Router		/api/v1/teams/{id}/members/{user_id} [patch]
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
