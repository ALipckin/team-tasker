package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New создаёт JSON-логгер для структурированных логов.
// Уровень задаётся переменной LOG_LEVEL (debug, info, warn, error).
func New() *slog.Logger {
	level := slog.LevelInfo

	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
