package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// App — структура приложения.
// Содержит DI-контейнер и HTTP-сервер.
// API-слой отвечает за хендлеры и роутинг, а http.Server живёт здесь —
// это инфраструктура, а не бизнес-логика.
type App struct {
	diContainer *diContainer
	httpServer  *http.Server
}

// New создаёт приложение и инициализирует все зависимости через DI-контейнер.
// Вся цепочка зависимостей разрешается здесь: от базы данных до HTTP-хендлеров.
// Если какая-то зависимость не создалась — процесс завершится внутри контейнера.
func New() *App {
	a := &App{
		diContainer: newDIContainer(),
	}

	a.initDeps()

	return a
}

// initDeps последовательно вызывает функции инициализации.
// Если нужно добавить новый шаг (миграции, метрики и т.д.) —
// просто добавить функцию в слайс inits.
func (a *App) initDeps() {
	inits := []func(){
		a.initHTTPServer,
	}

	for _, fn := range inits {
		fn()
	}
}

// initHTTPServer создаёт HTTP-сервер.
// API-слой (Handler) предоставляет только роутинг — Routes().
// А http.Server с адресом и конфигом создаётся здесь, в app-слое.
func (a *App) initHTTPServer() {
	a.httpServer = &http.Server{
		Addr:              ":" + a.diContainer.Config().AppPort,
		Handler:           a.diContainer.Handler().Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Run запускает HTTP-сервер и ждёт SIGINT/SIGTERM.
// После сигнала дожидается завершения активных запросов и закрывает БД и кэш.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("сервер запущен", "addr", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		a.diContainer.Close()
		return err
	case <-ctx.Done():
	}

	slog.Info("останавливаем сервер")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := a.httpServer.Shutdown(shutdownCtx)
	a.diContainer.Close()

	return err
}
