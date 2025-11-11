package config

import (
	httpAdapter "github.com/rostislaved/go-clean-architecture/internal/app/adapters/primary/http-adapter"
	openvpn_adapter "github.com/rostislaved/go-clean-architecture/internal/app/adapters/secondary/openvpn"
)

type Adapters struct {
	Primary   Primary
	Secondary Secondary
}

type Primary struct {
	HttpAdapter httpAdapter.Config
}

type Secondary struct {
	OpenVPN openvpn_adapter.Config
}
