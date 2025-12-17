package internal

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            string
	OpenVPNBaseDir  string
	OpenVPNOutputDir string
	ShutdownTimeout time.Duration
}

// LoadConfig loads configuration from config.toml if it exists, otherwise uses defaults
func LoadConfig() (Config, error) {
	cfg := Config{
		Port:             ":8080",
		OpenVPNBaseDir:   "/data/openvpn/server",
		OpenVPNOutputDir: "/data/openvpn/clients",
		ShutdownTimeout:  15 * time.Second,
	}

	data, err := parseConfigFile("config.toml")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	if port := getNestedString(data, "Adapters", "Primary", "HttpAdapter", "Server", "Port"); port != "" {
		cfg.Port = port
	}

	if baseDir := getNestedString(data, "Adapters", "Secondary", "OpenVPN", "BaseDir"); baseDir != "" {
		cfg.OpenVPNBaseDir = baseDir
	}

	if outputDir := getNestedString(data, "Adapters", "Secondary", "OpenVPN", "OutputDir"); outputDir != "" {
		cfg.OpenVPNOutputDir = outputDir
	}

	return cfg, nil
}

func parseConfigFile(path string) (map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	root := make(map[string]any)
	var currentPath []string

	for scanner.Scan() {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(line[1 : len(line)-1])
			currentPath = strings.Split(section, ".")
			continue
		}

		key, value, err := parseAssignment(line)
		if err != nil {
			continue
		}

		assignValue(root, currentPath, key, value)
	}

	return root, scanner.Err()
}

func stripComment(line string) string {
	if idx := strings.Index(line, "#"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func parseAssignment(line string) (string, any, error) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", nil, errors.New("invalid assignment")
	}

	key := strings.TrimSpace(parts[0])
	valueRaw := strings.TrimSpace(parts[1])

	if strings.HasPrefix(valueRaw, "\"") {
		value, _ := strconv.Unquote(valueRaw)
		return key, value, nil
	}

	if i, err := strconv.ParseInt(valueRaw, 10, 64); err == nil {
		return key, i, nil
	}

	return key, valueRaw, nil
}

func assignValue(root map[string]any, path []string, key string, value any) {
	current := root
	for _, segment := range path {
		next, ok := current[segment]
		if !ok {
			child := make(map[string]any)
			current[segment] = child
			current = child
		} else if child, ok := next.(map[string]any); ok {
			current = child
		}
	}
	current[key] = value
}

func getNestedString(data map[string]any, keys ...string) string {
	current := data
	for i, key := range keys {
		if i == len(keys)-1 {
			if val, ok := current[key].(string); ok {
				return val
			}
			return ""
		}
		if next, ok := current[key].(map[string]any); ok {
			current = next
		} else {
			return ""
		}
	}
	return ""
}
