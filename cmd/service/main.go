package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/rostislaved/go-clean-architecture/internal/app"
	"github.com/rostislaved/go-clean-architecture/internal/app/config"
)

func main() {
	cfg := config.New()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	application := app.New(logger, cfg)

	if err := application.Start(context.Background()); err != nil {
		logger.Error("service stopped", "error", err.Error())
		os.Exit(1)
	}
}
