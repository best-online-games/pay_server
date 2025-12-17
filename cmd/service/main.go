package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/rostislaved/go-clean-architecture/internal"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	logger.Info("bootstrapping pay_server service")

	cfg, err := internal.LoadConfig()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("config loaded",
		"http_port", cfg.Port,
		"openvpn_base_dir", cfg.OpenVPNBaseDir,
		"openvpn_output_dir", cfg.OpenVPNOutputDir,
	)

	server := internal.NewServer(logger, cfg)

	logger.Info("server constructed, starting")

	if err := server.Start(context.Background()); err != nil {
		logger.Error("service stopped", "error", err.Error())
		os.Exit(1)
	}
}
