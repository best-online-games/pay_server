package app

import (
	"context"
	"log/slog"

	http_adapter "github.com/rostislaved/go-clean-architecture/internal/app/adapters/primary/http-adapter"
	"github.com/rostislaved/go-clean-architecture/internal/app/adapters/secondary/openvpn"
	"github.com/rostislaved/go-clean-architecture/internal/app/application/usecases"
	"github.com/rostislaved/go-clean-architecture/internal/app/config"
)

type App struct {
	httpAdapter *http_adapter.HttpAdapter
}

func New(l *slog.Logger, cfg config.Config) App {
	openvpnManager := openvpn.New(l, cfg.Adapters.Secondary.OpenVPN)

	usecases := usecases.New(
		l,
		openvpnManager,
	)

	httpAdapter := http_adapter.New(l, cfg.Adapters.Primary.HttpAdapter, usecases)

	return App{
		httpAdapter: httpAdapter,
	}
}

func (a App) Start(ctx context.Context) error {
	return a.httpAdapter.Start(ctx)
}
