package openvpn

import "errors"

var (
	ErrInvalidClientName    = errors.New("openvpn: invalid client name")
	ErrClientNotFound       = errors.New("openvpn: client not found")
	ErrClientAlreadyRevoked = errors.New("openvpn: client already revoked")
)
