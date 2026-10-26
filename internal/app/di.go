package app

import (
	"log/slog"
	"os"

	"backend/internal/api"
	"backend/internal/cache"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/repository"
	"backend/internal/service"
)

type diContainer struct {
	cfg *config.Config

	db    database.DB
	cache cache.Cache

	userRepo    repository.UserRepo
	teamRepo    repository.TeamRepo
	taskRepo    repository.TaskRepo
	historyRepo repository.HistoryRepo
	commentRepo repository.CommentRepo
	statsRepo   repository.StatsRepo

	authService  service.AuthService
	teamService  service.TeamService
	taskService  service.TaskService
	statsService service.StatsService

	handler api.Handler
}

func newDIContainer() *diContainer {
	return &diContainer{}
}

func (d *diContainer) Config() *config.Config {
	if d.cfg == nil {
		cfg, err := config.AppConfig()
		if err != nil {
			slog.Error("не удалось загрузить конфиг", "err", err)
			os.Exit(1)
		}

		d.cfg = cfg
	}

	return d.cfg
}

func (d *diContainer) DB() database.DB {
	if d.db == nil {
		db, err := database.New(d.Config().DSN, d.Config().MigrationsPath)
		if err != nil {
			slog.Error("не удалось подключиться к БД", "err", err)
			os.Exit(1)
		}

		d.db = db
	}

	return d.db
}

func (d *diContainer) Cache() cache.Cache {
	if d.cache == nil {
		cfg := d.Config()
		d.cache = cache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	}

	return d.cache
}

func (d *diContainer) UserRepo() repository.UserRepo {
	if d.userRepo == nil {
		d.userRepo = repository.NewUserRepo(d.DB())
	}

	return d.userRepo
}

func (d *diContainer) TeamRepo() repository.TeamRepo {
	if d.teamRepo == nil {
		d.teamRepo = repository.NewTeamRepo(d.DB())
	}

	return d.teamRepo
}

func (d *diContainer) TaskRepo() repository.TaskRepo {
	if d.taskRepo == nil {
		d.taskRepo = repository.NewTaskRepo(d.DB())
	}

	return d.taskRepo
}

func (d *diContainer) HistoryRepo() repository.HistoryRepo {
	if d.historyRepo == nil {
		d.historyRepo = repository.NewHistoryRepo(d.DB())
	}

	return d.historyRepo
}

func (d *diContainer) CommentRepo() repository.CommentRepo {
	if d.commentRepo == nil {
		d.commentRepo = repository.NewCommentRepo(d.DB())
	}

	return d.commentRepo
}

func (d *diContainer) StatsRepo() repository.StatsRepo {
	if d.statsRepo == nil {
		d.statsRepo = repository.NewStatsRepo(d.DB())
	}

	return d.statsRepo
}

func (d *diContainer) AuthService() service.AuthService {
	if d.authService == nil {
		d.authService = service.NewAuthService(d.UserRepo(), d.Config().JWTSecret, d.Config().JWTTTL)
	}

	return d.authService
}

func (d *diContainer) TeamService() service.TeamService {
	if d.teamService == nil {
		d.teamService = service.NewTeamService(d.DB(), d.TeamRepo(), d.UserRepo())
	}

	return d.teamService
}

func (d *diContainer) TaskService() service.TaskService {
	if d.taskService == nil {
		d.taskService = service.NewTaskService(
			d.DB(),
			d.TaskRepo(),
			d.TeamRepo(),
			d.HistoryRepo(),
			d.CommentRepo(),
			d.Cache(),
			d.Config().TasksCacheTTL,
		)
	}

	return d.taskService
}

func (d *diContainer) StatsService() service.StatsService {
	if d.statsService == nil {
		d.statsService = service.NewStatsService(d.TeamRepo(), d.StatsRepo())
	}

	return d.statsService
}

func (d *diContainer) Handler() api.Handler {
	if d.handler == nil {
		d.handler = api.NewHandler(
			d.AuthService(),
			d.TeamService(),
			d.TaskService(),
			d.StatsService(),
			api.Options{
				RateLimitRPS:   d.Config().RateLimitRPS,
				RateLimitBurst: d.Config().RateLimitBurst,
			},
		)
	}

	return d.handler
}

// Close закрывает внешние подключения в обратном порядке создания.
func (d *diContainer) Close() {
	if d.cache != nil {
		if err := d.cache.Close(); err != nil {
			slog.Error("ошибка закрытия кэша", "err", err)
		}
	}

	if d.db != nil {
		if err := d.db.Close(); err != nil {
			slog.Error("ошибка закрытия БД", "err", err)
		}
	}
}
