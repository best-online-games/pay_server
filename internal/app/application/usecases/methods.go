package usecases

import (
	"context"
	"errors"
)

func (uc *UseCases) EnsureOpenVPNClient(ctx context.Context, name string) (string, error) {
	if uc.openvpnPortal == nil {
		return "", errors.New("openvpn manager is not configured")
	}

	cfg, err := uc.openvpnPortal.EnsureClientConfig(ctx, name)
	if err != nil {
		uc.logger.Error("ensure client failed", "client", name, "error", err)
		return "", err
	}

	uc.logger.Info("ensure client succeeded", "client", name)
	return cfg, nil
}

func (uc *UseCases) RevokeOpenVPNClient(ctx context.Context, name string) error {
	if uc.openvpnPortal == nil {
		return errors.New("openvpn manager is not configured")
	}

	if err := uc.openvpnPortal.RevokeClient(ctx, name); err != nil {
		uc.logger.Error("revoke client failed", "client", name, "error", err)
		return err
	}

	uc.logger.Info("revoke client succeeded", "client", name)
	return nil
}
