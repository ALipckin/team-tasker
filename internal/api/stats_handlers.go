package api

import (
	"net/http"
)

// teamStats godoc
//
//	@Summary	SQL-отчёт по команде (owner или admin)
//	@Tags		teams
//	@Produce	json
//	@Security	BearerAuth
//	@Param		team_id	path		int	true	"ID команды"
//	@Success	200		{object}	domain.TeamStats
//	@Failure	401		{object}	errorResponse	"Нет или неверный токен"
//	@Failure	403		{object}	errorResponse	"Недостаточно прав"
//	@Router		/api/v1/teams/{team_id}/stats [get]
func (h *handler) teamStats(w http.ResponseWriter, r *http.Request) {
	teamID, err := pathID(r, "team_id")
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
