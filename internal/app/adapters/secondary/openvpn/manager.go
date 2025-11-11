package openvpn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	openvpn_domain "github.com/rostislaved/go-clean-architecture/internal/app/domain/openvpn"
)

type Manager struct {
	logger *slog.Logger
	cfg    Config

	mu sync.Mutex
}

func New(logger *slog.Logger, cfg Config) *Manager {
	cfg.normalize()

	return &Manager{
		logger: logger,
		cfg:    cfg,
	}
}

// EnsureClientConfig returns the OpenVPN client configuration for the provided
// name. If the client does not exist, a new certificate is created using the
// same procedure as openvpn-install.sh, stored on disk and returned.
func (m *Manager) EnsureClientConfig(ctx context.Context, rawName string) (string, error) {
	clientName, err := sanitizeName(rawName)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if data, err := os.ReadFile(m.cfg.clientConfigPath(clientName)); err == nil {
		return string(data), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read client config: %w", err)
	}

	status, err := m.lookupClientStatus(clientName)
	if err != nil && !errors.Is(err, openvpn_domain.ErrClientNotFound) {
		return "", err
	}

	if status == clientStatusRevoked {
		if err := m.removeClientArtifacts(clientName); err != nil {
			return "", err
		}
	}

	if status != clientStatusValid {
		if err := m.buildClientCertificate(ctx, clientName); err != nil {
			return "", err
		}
	}

	configText, err := m.composeClientConfig(clientName)
	if err != nil {
		return "", err
	}

	if err := m.writeClientConfig(clientName, configText); err != nil {
		return "", err
	}

	return configText, nil
}

// RevokeClient revokes an existing certificate and refreshes the CRL.
func (m *Manager) RevokeClient(ctx context.Context, rawName string) error {
	clientName, err := sanitizeName(rawName)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	status, err := m.lookupClientStatus(clientName)
	if err != nil {
		return err
	}

	if status == clientStatusRevoked {
		return openvpn_domain.ErrClientAlreadyRevoked
	}

	if err := m.runEasyRSA(ctx, "--batch", "revoke", clientName); err != nil {
		return err
	}

	if err := m.runEasyRSA(ctx, "--batch", "--days=3650", "gen-crl"); err != nil {
		return err
	}

	if err := m.refreshCRL(); err != nil {
		return err
	}

	if err := os.Remove(m.cfg.clientConfigPath(clientName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove cached config: %w", err)
	}

	return nil
}

const (
	clientStatusValid   = "V"
	clientStatusRevoked = "R"
)

func (m *Manager) lookupClientStatus(name string) (string, error) {
	indexPath := filepath.Join(m.cfg.easyRSADir(), "pki", "index.txt")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", openvpn_domain.ErrClientNotFound
		}

		return "", fmt.Errorf("read index.txt: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var lastMatch string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.Contains(line, "/CN="+name) {
			lastMatch = line
		}
	}

	if lastMatch == "" {
		return "", openvpn_domain.ErrClientNotFound
	}

	return string(lastMatch[0]), nil
}

func (m *Manager) buildClientCertificate(ctx context.Context, name string) error {
	args := []string{"--batch", "--days=3650", "build-client-full", name, "nopass"}
	if err := m.runEasyRSA(ctx, args...); err != nil {
		return err
	}

	return nil
}

func (m *Manager) runEasyRSA(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "./easyrsa", args...)
	cmd.Dir = m.cfg.easyRSADir()

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		m.logger.Error("easyrsa command failed", "args", args, "output", output.String(), "error", err)

		return fmt.Errorf("easyrsa %v: %w: %s", args, err, output.String())
	}

	return nil
}

func (m *Manager) composeClientConfig(name string) (string, error) {
	var builder strings.Builder

	if err := m.appendFile(&builder, m.cfg.clientCommonPath()); err != nil {
		return "", err
	}

	if err := m.appendWrappedFile(&builder, "<ca>\n", "\n</ca>\n", m.cfg.caCertPath()); err != nil {
		return "", err
	}

	if err := m.appendWrappedFile(&builder, "<cert>\n", "\n</cert>\n", m.cfg.issuedCertPath(name), "-----BEGIN CERTIFICATE-----"); err != nil {
		return "", err
	}

	if err := m.appendWrappedFile(&builder, "<key>\n", "\n</key>\n", m.cfg.privateKeyPath(name)); err != nil {
		return "", err
	}

	if err := m.appendWrappedFile(&builder, "<tls-crypt>\n", "\n</tls-crypt>\n", m.cfg.tlsAuthKeyPath(), "-----BEGIN OpenVPN Static key-----"); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func (m *Manager) appendFile(builder *strings.Builder, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	builder.Write(data)
	if len(data) == 0 || data[len(data)-1] != '\n' {
		builder.WriteByte('\n')
	}

	return nil
}

func (m *Manager) appendWrappedFile(builder *strings.Builder, prefix, suffix, path string, markers ...string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	if len(markers) > 0 {
		if trimmed := tailFromMarker(data, markers[0]); trimmed != nil {
			data = trimmed
		}
	}

	builder.WriteString(prefix)
	builder.Write(data)
	if len(data) == 0 || data[len(data)-1] != '\n' {
		builder.WriteByte('\n')
	}
	builder.WriteString(suffix)

	return nil
}

func tailFromMarker(data []byte, marker string) []byte {
	idx := bytes.Index(data, []byte(marker))
	if idx == -1 {
		return data
	}

	return data[idx:]
}

func (m *Manager) writeClientConfig(name, contents string) error {
	path := m.cfg.clientConfigPath(name)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ensure client config dir: %w", err)
	}

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return fmt.Errorf("write client config: %w", err)
	}

	return nil
}

func (m *Manager) removeClientArtifacts(name string) error {
	paths := []string{
		m.cfg.issuedCertPath(name),
		m.cfg.privateKeyPath(name),
		m.cfg.requestPath(name),
	}

	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}

	return nil
}

func (m *Manager) refreshCRL() error {
	source := m.cfg.crlSourcePath()
	target := m.cfg.crlTargetPath()

	var uid = -1
	var gid = -1

	if info, err := os.Stat(target); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid = int(stat.Uid)
			gid = int(stat.Gid)
		}
	}

	if err := copyFile(source, target, 0o640); err != nil {
		return fmt.Errorf("copy CRL: %w", err)
	}

	if uid != -1 || gid != -1 {
		if err := os.Chown(target, uid, gid); err != nil {
			return fmt.Errorf("chown CRL: %w", err)
		}
	}

	return nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	tmpPath := dst + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return err
	}

	return nil
}

func sanitizeName(input string) (string, error) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return "", openvpn_domain.ErrInvalidClientName
	}

	var builder strings.Builder
	for _, r := range clean {
		if isAllowedRune(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}

	name := builder.String()
	if name == "" {
		return "", openvpn_domain.ErrInvalidClientName
	}

	return name, nil
}

func isAllowedRune(r rune) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	return r == '-' || r == '_'
}
