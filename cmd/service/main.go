package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/rostislaved/go-clean-architecture/internal/app"
	"github.com/rostislaved/go-clean-architecture/internal/app/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	logger.Info("bootstrapping pay_server service")

	cfg, loadedFromFile, err := config.New()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if loadedFromFile {
		logger.Info("config loaded",
			"path", "config.toml",
			"http_port", cfg.Adapters.Primary.HttpAdapter.Server.Port,
		)
	} else {
		logger.Warn("config.toml not found, using built-in defaults",
			"http_port", cfg.Adapters.Primary.HttpAdapter.Server.Port,
		)
	}

	application := app.New(logger, cfg)

	logger.Info("application constructed, starting adapters")

	if err := application.Start(context.Background()); err != nil {
		logger.Error("service stopped", "error", err.Error())
		os.Exit(1)
	}
}
