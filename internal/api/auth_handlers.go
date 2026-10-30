package api

import (
	"net/http"

	"backend/internal/domain"
)

type registerRequest struct {
	Email    string `json:"email" validate:"required" format:"email" example:"alice@example.com"`
	Password string `json:"password" validate:"required" minLength:"8" maxLength:"72" example:"secret123"`
	Name     string `json:"name" validate:"required" example:"Alice"`
}

// register godoc
//
//	@Summary	Регистрация пользователя
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		registerRequest	true	"Данные пользователя"
//	@Success	201		{object}	domain.User
//	@Failure	400		{object}	errorResponse	"Некорректный запрос"
//	@Failure	409		{object}	errorResponse	"Пользователь уже существует"
//	@Failure	429		{object}	errorResponse	"Превышен лимит запросов"
//	@Router		/api/v1/register [post]
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
	Email    string `json:"email" validate:"required" format:"email" example:"alice@example.com"`
	Password string `json:"password" validate:"required" example:"secret123"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  domain.User `json:"user"`
}

// login godoc
//
//	@Summary	Аутентификация, выдача JWT
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		loginRequest	true	"Учётные данные"
//	@Success	200		{object}	loginResponse
//	@Failure	400		{object}	errorResponse	"Некорректный запрос"
//	@Failure	401		{object}	errorResponse	"Неверные учётные данные"
//	@Router		/api/v1/login [post]
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
