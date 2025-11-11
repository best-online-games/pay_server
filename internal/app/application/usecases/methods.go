package usecases

import (
	"context"
	"errors"
)

func (uc *UseCases) EnsureOpenVPNClient(ctx context.Context, name string) (string, error) {
	if uc.openvpnPortal == nil {
		return "", errors.New("openvpn manager is not configured")
	}

	return uc.openvpnPortal.EnsureClientConfig(ctx, name)
}

func (uc *UseCases) RevokeOpenVPNClient(ctx context.Context, name string) error {
	if uc.openvpnPortal == nil {
		return errors.New("openvpn manager is not configured")
	}

	return uc.openvpnPortal.RevokeClient(ctx, name)
}
