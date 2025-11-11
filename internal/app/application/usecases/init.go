package usecases

import (
	"context"
	"log/slog"
)

type UseCases struct {
	logger        *slog.Logger
	openvpnPortal openvpnManager
}

type openvpnManager interface {
	EnsureClientConfig(ctx context.Context, client string) (string, error)
	RevokeClient(ctx context.Context, client string) error
}

func New(
	l *slog.Logger,
	openvpnManager openvpnManager,
) *UseCases {
	return &UseCases{
		logger:        l,
		openvpnPortal: openvpnManager,
	}
}
