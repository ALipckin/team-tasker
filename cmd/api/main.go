package main

import (
	"log/slog"
	"os"

	"backend/internal/app"
	"backend/internal/logger"
)

func main() {
	slog.SetDefault(logger.New())

	a := app.New()

	if err := a.Run(); err != nil {
		slog.Error("application error", "err", err)
		os.Exit(1)
	}
}
