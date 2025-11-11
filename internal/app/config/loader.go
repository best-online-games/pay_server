package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FromFile loads the application config from the provided path.
func FromFile(path string) (Config, error) {
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}

	raw, err := parseSimpleTOML(path)
	if err != nil {
		return Config{}, err
	}

	cfg, err := buildConfig(raw)
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func parseSimpleTOML(path string) (map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	root := make(map[string]any)
	var currentPath []string

	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(line[1 : len(line)-1])
			if section == "" {
				return nil, fmt.Errorf("config: %s:%d empty section name", path, lineNo)
			}

			parts := strings.Split(section, ".")
			currentPath = currentPath[:0]
			for _, part := range parts {
				p := strings.TrimSpace(part)
				if p == "" {
					return nil, fmt.Errorf("config: %s:%d invalid section path", path, lineNo)
				}
				currentPath = append(currentPath, p)
			}
			continue
		}

		key, value, err := parseAssignment(line)
		if err != nil {
			return nil, fmt.Errorf("config: %s:%d %w", path, lineNo, err)
		}

		if err := assignValue(root, currentPath, key, value); err != nil {
			return nil, fmt.Errorf("config: %s:%d %w", path, lineNo, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}

	return root, nil
}

func stripComment(line string) string {
	var (
		inString bool
		escape   bool
		builder  strings.Builder
	)

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if ch == '"' && !escape {
			inString = !inString
		}

		if !inString && ch == '#' {
			break
		}

		builder.WriteByte(ch)

		if ch == '\\' && inString && !escape {
			escape = true
			continue
		}

		if escape {
			escape = false
		}
	}

	return builder.String()
}

func parseAssignment(line string) (string, any, error) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", nil, errors.New("expected key = value pair")
	}

	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", nil, errors.New("empty key")
	}

	valueRaw := strings.TrimSpace(parts[1])
	if valueRaw == "" {
		return "", nil, fmt.Errorf("missing value for key %q", key)
	}

	value, err := parseValue(valueRaw)
	if err != nil {
		return "", nil, err
	}

	return key, value, nil
}

func parseValue(raw string) (any, error) {
	if strings.HasPrefix(raw, "\"") {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid string literal: %w", err)
		}
		return value, nil
	}

	if raw == "true" || raw == "false" {
		return raw == "true", nil
	}

	if strings.Contains(raw, "_") {
		raw = strings.ReplaceAll(raw, "_", "")
	}

	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return i, nil
	}

	return nil, fmt.Errorf("unsupported value %q", raw)
}

func assignValue(root map[string]any, path []string, key string, value any) error {
	current := root

	for _, segment := range path {
		next, ok := current[segment]
		if !ok {
			child := make(map[string]any)
			current[segment] = child
			current = child
			continue
		}

		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("section %q already holds a value", strings.Join(path, "."))
		}

		current = child
	}

	current[key] = value
	return nil
}

func buildConfig(data map[string]any) (Config, error) {
	var cfg Config

	adapters, err := getMap(data, "Adapters")
	if err != nil {
		return cfg, err
	}
	if adapters == nil {
		return cfg, errors.New("config: section Adapters is required")
	}

	if err := decodePrimary(&cfg, adapters); err != nil {
		return cfg, err
	}

	if err := decodeSecondary(&cfg, adapters); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func decodePrimary(cfg *Config, adapters map[string]any) error {
	primary, err := getMap(adapters, "Primary")
	if err != nil {
		return err
	}
	if primary == nil {
		return errors.New("config: section Adapters.Primary is required")
	}

	httpAdapter, err := getMap(primary, "HttpAdapter")
	if err != nil {
		return err
	}
	if httpAdapter == nil {
		return errors.New("config: section Adapters.Primary.HttpAdapter is required")
	}

	server, err := getMap(httpAdapter, "Server")
	if err != nil {
		return err
	}
	if server == nil {
		return errors.New("config: section Adapters.Primary.HttpAdapter.Server is required")
	}

	port, err := getString(server, "Port")
	if err != nil {
		return err
	}
	if port == "" {
		return errors.New("config: Adapters.Primary.HttpAdapter.Server.Port is required")
	}
	cfg.Adapters.Primary.HttpAdapter.Server.Port = port

	if startMsg, err := getString(server, "StartMsg"); err != nil {
		return err
	} else {
		cfg.Adapters.Primary.HttpAdapter.Server.StartMsg = startMsg
	}

	timeoutFields := []struct {
		key string
		dst *time.Duration
	}{
		{"ReadTimeout", &cfg.Adapters.Primary.HttpAdapter.Server.ReadTimeout},
		{"WriteTimeout", &cfg.Adapters.Primary.HttpAdapter.Server.WriteTimeout},
		{"ReadHeaderTimeout", &cfg.Adapters.Primary.HttpAdapter.Server.ReadHeaderTimeout},
		{"ShutdownTimeout", &cfg.Adapters.Primary.HttpAdapter.Server.ShutdownTimeout},
	}

	for _, field := range timeoutFields {
		value, err := getDuration(server, field.key)
		if err != nil {
			return err
		}
		*field.dst = value
	}

	routerMap, err := getMap(httpAdapter, "Router")
	if err != nil {
		return err
	}

	if routerMap != nil {
		authCfg, err := getString(routerMap, "AuthenticationConfig")
		if err != nil {
			return err
		}
		cfg.Adapters.Primary.HttpAdapter.Router.AuthenticationConfig = authCfg

		authzCfg, err := getString(routerMap, "AuthorizationConfig")
		if err != nil {
			return err
		}
		cfg.Adapters.Primary.HttpAdapter.Router.AuthorizationConfig = authzCfg

		if shutdown, err := getMap(routerMap, "Shutdown"); err != nil {
			return err
		} else if shutdown != nil {
			duration, err := getDuration(shutdown, "Duration")
			if err != nil {
				return err
			}
			cfg.Adapters.Primary.HttpAdapter.Router.Shutdown.Duration = duration
		}

		if timeout, err := getMap(routerMap, "Timeout"); err != nil {
			return err
		} else if timeout != nil {
			duration, err := getDuration(timeout, "Duration")
			if err != nil {
				return err
			}
			cfg.Adapters.Primary.HttpAdapter.Router.Timeout.Duration = duration
		}
	}

	return nil
}

func decodeSecondary(cfg *Config, adapters map[string]any) error {
	secondary, err := getMap(adapters, "Secondary")
	if err != nil {
		return err
	}
	if secondary == nil {
		return nil
	}

	openvpnSection, err := getMap(secondary, "OpenVPN")
	if err != nil {
		return err
	}
	if openvpnSection == nil {
		return nil
	}

	if baseDir, err := getString(openvpnSection, "BaseDir"); err != nil {
		return err
	} else {
		cfg.Adapters.Secondary.OpenVPN.BaseDir = baseDir
	}

	if outputDir, err := getString(openvpnSection, "OutputDir"); err != nil {
		return err
	} else {
		cfg.Adapters.Secondary.OpenVPN.OutputDir = outputDir
	}

	if scriptPath, err := getString(openvpnSection, "ScriptPath"); err != nil {
		return err
	} else {
		cfg.Adapters.Secondary.OpenVPN.ScriptPath = scriptPath
	}

	return nil
}

func getMap(parent map[string]any, key string) (map[string]any, error) {
	if parent == nil {
		return nil, nil
	}

	val, ok := parent[key]
	if !ok {
		return nil, nil
	}

	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("config: section %s has invalid type %T", key, val)
	}

	return m, nil
}

func getString(parent map[string]any, key string) (string, error) {
	if parent == nil {
		return "", nil
	}

	val, ok := parent[key]
	if !ok || val == nil {
		return "", nil
	}

	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("config: field %s must be a string, got %T", key, val)
	}

	return str, nil
}

func getDuration(parent map[string]any, key string) (time.Duration, error) {
	if parent == nil {
		return 0, nil
	}

	val, ok := parent[key]
	if !ok || val == nil {
		return 0, nil
	}

	switch v := val.(type) {
	case string:
		if v == "" {
			return 0, nil
		}
		duration, err := time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("config: invalid duration %s: %w", key, err)
		}
		return duration, nil
	case int64:
		return time.Duration(v) * time.Second, nil
	default:
		return 0, fmt.Errorf("config: field %s must be a duration string or integer seconds, got %T", key, val)
	}
}
