package config

import (
	"errors"
	"os"
	"time"
)

const defaultConfigPath = "config.toml"

// New loads configuration from config.toml if it exists, otherwise falls back
// to sensible defaults so the service can still boot (useful in containers
// where config.toml is not packaged with the binary).
func New() (Config, bool, error) {
	cfg, err := FromFile(defaultConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultConfig(), false, nil
		}

		return Config{}, false, err
	}

	return cfg, true, nil
}

func defaultConfig() Config {
	cfg := Config{}

	cfg.Adapters.Primary.HttpAdapter.Server.Port = ":8080"
	cfg.Adapters.Primary.HttpAdapter.Server.StartMsg = "http server listening"
	cfg.Adapters.Primary.HttpAdapter.Server.ReadHeaderTimeout = 5 * time.Second
	cfg.Adapters.Primary.HttpAdapter.Server.ReadTimeout = 60 * time.Second
	cfg.Adapters.Primary.HttpAdapter.Server.WriteTimeout = 60 * time.Second
	cfg.Adapters.Primary.HttpAdapter.Server.ShutdownTimeout = 15 * time.Second

	cfg.Adapters.Primary.HttpAdapter.Router.Shutdown.Duration = 15 * time.Second
	cfg.Adapters.Primary.HttpAdapter.Router.Timeout.Duration = 60 * time.Second

	cfg.Adapters.Secondary.OpenVPN.BaseDir = "/data/openvpn/server"
	cfg.Adapters.Secondary.OpenVPN.OutputDir = "/data/openvpn/clients"
	cfg.Adapters.Secondary.OpenVPN.ScriptPath = "./openvpn-install.sh"

	return cfg
}
