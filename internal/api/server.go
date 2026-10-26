// Package api — HTTP-слой сервиса.
//
//	@title			Task Service API
//	@version		1.0.0
//	@description	REST API для управления задачами внутри команд.
//	@description
//	@description	Роли в команде:
//	@description	- **owner** — создатель: полный доступ, приглашает участников и администраторов, меняет роли.
//	@description	- **admin** — приглашает участников (role=member), редактирует любые задачи, видит аналитику.
//	@description	- **member** — видит задачи команды, создаёт задачи, редактирует свои задачи;
//	@description	исполнитель задачи может менять только её статус.
//	@description
//	@description				Все защищённые эндпоинты требуют заголовок `Authorization: Bearer <JWT>`.
//	@host						localhost:8080
//	@BasePath					/
//	@schemes					http
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Значение в формате `Bearer <JWT>`.
package api

import (
	"context"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	"backend/internal/domain"
	"backend/internal/service"
)

type AuthService interface {
	Register(ctx context.Context, email, password, name string) (domain.User, error)
	Login(ctx context.Context, email, password string) (string, domain.User, error)
	ParseToken(token string) (int64, error)
}

type TeamService interface {
	Create(ctx context.Context, userID int64, name string) (domain.Team, error)
	List(ctx context.Context, userID int64) ([]domain.Team, error)
	Invite(ctx context.Context, actorID, teamID int64, email string, role domain.Role) (domain.TeamMember, error)
	ChangeRole(ctx context.Context, actorID, teamID, userID int64, role domain.Role) (domain.TeamMember, error)
}

type TaskService interface {
	Create(ctx context.Context, userID int64, in service.CreateTaskInput) (domain.Task, error)
	Get(ctx context.Context, userID, taskID int64) (domain.Task, error)
	List(ctx context.Context, userID int64, f domain.TaskFilter) (domain.TaskList, error)
	Update(ctx context.Context, userID, taskID int64, in service.UpdateTaskInput) (domain.Task, error)
	History(ctx context.Context, userID, taskID int64) ([]domain.TaskHistory, error)
	AddComment(ctx context.Context, userID, taskID int64, content string) (domain.TaskComment, error)
	Comments(ctx context.Context, userID, taskID int64) ([]domain.TaskComment, error)
}

type StatsService interface {
	TeamStats(ctx context.Context, userID, teamID int64) (domain.TeamStats, error)
}

type Handler interface {
	Routes() http.Handler
}

type Options struct {
	RateLimitRPS   float64
	RateLimitBurst int
	// TrustedProxies — сети прокси, которым доверяем X-Forwarded-For.
	TrustedProxies []*net.IPNet
}

type handler struct {
	authService  AuthService
	teamService  TeamService
	taskService  TaskService
	statsService StatsService
	opts         Options
}

func NewHandler(
	authService AuthService,
	teamService TeamService,
	taskService TaskService,
	statsService StatsService,
	opts Options,
) Handler {
	return &handler{
		authService:  authService,
		teamService:  teamService,
		taskService:  taskService,
		statsService: statsService,
		opts:         opts,
	}
}

func (h *handler) Routes() http.Handler {
	r := mux.NewRouter()
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "route not found")
	})
	r.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	r.Use(realIP(h.opts.TrustedProxies), requestLogger, recoverer)
	if h.opts.RateLimitRPS > 0 {
		r.Use(newRateLimiter(h.opts.RateLimitRPS, h.opts.RateLimitBurst).middleware)
	}

	r.HandleFunc("/health", h.healthHandler).Methods(http.MethodGet)
	r.HandleFunc("/swagger", swaggerUIHandler).Methods(http.MethodGet)
	r.HandleFunc("/swagger/", swaggerUIHandler).Methods(http.MethodGet)
	r.HandleFunc("/swagger/doc.json", swaggerSpecHandler).Methods(http.MethodGet)

	v1 := r.PathPrefix("/api/v1").Subrouter()
	v1.HandleFunc("/register", h.register).Methods(http.MethodPost)
	v1.HandleFunc("/login", h.login).Methods(http.MethodPost)

	private := v1.NewRoute().Subrouter()
	private.Use(h.authMiddleware)

	private.HandleFunc("/teams", h.createTeam).Methods(http.MethodPost)
	private.HandleFunc("/teams", h.listTeams).Methods(http.MethodGet)
	private.HandleFunc("/teams/{id:[0-9]+}/invite", h.inviteMember).Methods(http.MethodPost)
	private.HandleFunc("/teams/{id:[0-9]+}/members/{user_id:[0-9]+}", h.changeMemberRole).Methods(http.MethodPatch)
	private.HandleFunc("/teams/{id:[0-9]+}/stats", h.teamStats).Methods(http.MethodGet)

	private.HandleFunc("/tasks", h.createTask).Methods(http.MethodPost)
	private.HandleFunc("/tasks", h.listTasks).Methods(http.MethodGet)
	private.HandleFunc("/tasks/{id:[0-9]+}", h.getTask).Methods(http.MethodGet)
	private.HandleFunc("/tasks/{id:[0-9]+}", h.updateTask).Methods(http.MethodPut)
	private.HandleFunc("/tasks/{id:[0-9]+}/history", h.taskHistory).Methods(http.MethodGet)
	private.HandleFunc("/tasks/{id:[0-9]+}/comments", h.addComment).Methods(http.MethodPost)
	private.HandleFunc("/tasks/{id:[0-9]+}/comments", h.listComments).Methods(http.MethodGet)

	return r
}

// healthHandler godoc
//
//	@Summary	Health check
//	@Tags		system
//	@Produce	json
//	@Success	200	{object}	map[string]string	"OK"
//	@Router		/health [get]
func (h *handler) healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
