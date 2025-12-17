package internal

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
)

var (
	ErrInvalidClientName    = errors.New("openvpn: invalid client name")
	ErrClientNotFound       = errors.New("openvpn: client not found")
	ErrClientAlreadyRevoked = errors.New("openvpn: client already revoked")
)

type OpenVPNManager struct {
	logger    *slog.Logger
	baseDir   string
	outputDir string
	mu        sync.Mutex
}

func NewOpenVPNManager(logger *slog.Logger, baseDir, outputDir string) *OpenVPNManager {
	if baseDir == "" {
		baseDir = "/data/openvpn/server"
	}
	if outputDir == "" {
		outputDir = "/data/openvpn/clients"
	}

	return &OpenVPNManager{
		logger:    logger,
		baseDir:   baseDir,
		outputDir: outputDir,
	}
}

func (m *OpenVPNManager) EnsureClientConfig(ctx context.Context, rawName string) (string, error) {
	clientName, err := sanitizeName(rawName)
	if err != nil {
		m.logger.Error("ensure client: invalid name", "raw", rawName, "error", err)
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	configPath := m.clientConfigPath(clientName)
	if data, err := os.ReadFile(configPath); err == nil {
		return string(data), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read client config: %w", err)
	}

	status, err := m.lookupClientStatus(clientName)
	if err != nil && !errors.Is(err, ErrClientNotFound) {
		return "", err
	}

	if status == "R" {
		if err := m.removeClientArtifacts(clientName); err != nil {
			return "", err
		}
	}

	if status != "V" {
		if err := m.buildClientCertificate(ctx, clientName); err != nil {
			m.logger.Error("ensure client: build certificate failed", "client", clientName, "error", err)
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

	m.logger.Info("ensure client config prepared", "client", clientName)
	return configText, nil
}

func (m *OpenVPNManager) RevokeClient(ctx context.Context, rawName string) error {
	clientName, err := sanitizeName(rawName)
	if err != nil {
		m.logger.Error("revoke client: invalid name", "raw", rawName, "error", err)
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	status, err := m.lookupClientStatus(clientName)
	if err != nil {
		return err
	}

	if status == "R" {
		m.logger.Warn("revoke client: already revoked", "client", clientName)
		return ErrClientAlreadyRevoked
	}

	if err := m.runEasyRSA(ctx, "--batch", "revoke", clientName); err != nil {
		m.logger.Error("revoke client: easyrsa revoke failed", "client", clientName, "error", err)
		return err
	}

	if err := m.runEasyRSA(ctx, "--batch", "--days=3650", "gen-crl"); err != nil {
		m.logger.Error("revoke client: easyrsa gen-crl failed", "client", clientName, "error", err)
		return err
	}

	if err := m.refreshCRL(); err != nil {
		m.logger.Error("revoke client: refresh CRL failed", "client", clientName, "error", err)
		return err
	}

	if err := os.Remove(m.clientConfigPath(clientName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove cached config: %w", err)
	}

	m.logger.Info("revoke client completed", "client", clientName)
	return nil
}

func (m *OpenVPNManager) lookupClientStatus(name string) (string, error) {
	indexPath := filepath.Join(m.baseDir, "easy-rsa", "pki", "index.txt")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		m.logger.Error("read index failed", "path", indexPath, "error", err)
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrClientNotFound
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
		m.logger.Warn("client not found in index", "client", name)
		return "", ErrClientNotFound
	}

	return string(lastMatch[0]), nil
}

func (m *OpenVPNManager) buildClientCertificate(ctx context.Context, name string) error {
	return m.runEasyRSA(ctx, "--batch", "--days=3650", "build-client-full", name, "nopass")
}

func (m *OpenVPNManager) runEasyRSA(ctx context.Context, args ...string) error {
	m.logger.Info("running easyrsa", "args", args)
	cmd := exec.CommandContext(ctx, "./easyrsa", args...)
	cmd.Dir = filepath.Join(m.baseDir, "easy-rsa")

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		m.logger.Error("easyrsa command failed", "args", args, "output", output.String(), "error", err)
		return fmt.Errorf("easyrsa %v: %w: %s", args, err, output.String())
	}

	m.logger.Info("easyrsa command succeeded", "args", args)
	return nil
}

func (m *OpenVPNManager) composeClientConfig(name string) (string, error) {
	var builder strings.Builder

	clientCommonPath := filepath.Join(m.baseDir, "client-common.txt")
	if err := m.appendFile(&builder, clientCommonPath); err != nil {
		return "", err
	}

	caCertPath := filepath.Join(m.baseDir, "easy-rsa", "pki", "ca.crt")
	if err := m.appendWrappedFile(&builder, "<ca>\n", "\n</ca>\n", caCertPath); err != nil {
		return "", err
	}

	issuedCertPath := filepath.Join(m.baseDir, "easy-rsa", "pki", "issued", name+".crt")
	if err := m.appendWrappedFile(&builder, "<cert>\n", "\n</cert>\n", issuedCertPath, "-----BEGIN CERTIFICATE-----"); err != nil {
		return "", err
	}

	privateKeyPath := filepath.Join(m.baseDir, "easy-rsa", "pki", "private", name+".key")
	if err := m.appendWrappedFile(&builder, "<key>\n", "\n</key>\n", privateKeyPath); err != nil {
		return "", err
	}

	tlsAuthKeyPath := filepath.Join(m.baseDir, "tc.key")
	if err := m.appendWrappedFile(&builder, "<tls-crypt>\n", "\n</tls-crypt>\n", tlsAuthKeyPath, "-----BEGIN OpenVPN Static key-----"); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func (m *OpenVPNManager) appendFile(builder *strings.Builder, path string) error {
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

func (m *OpenVPNManager) appendWrappedFile(builder *strings.Builder, prefix, suffix, path string, markers ...string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	if len(markers) > 0 {
		if idx := bytes.Index(data, []byte(markers[0])); idx != -1 {
			data = data[idx:]
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

func (m *OpenVPNManager) writeClientConfig(name, contents string) error {
	path := m.clientConfigPath(name)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ensure client config dir: %w", err)
	}

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return fmt.Errorf("write client config: %w", err)
	}

	return nil
}

func (m *OpenVPNManager) removeClientArtifacts(name string) error {
	paths := []string{
		filepath.Join(m.baseDir, "easy-rsa", "pki", "issued", name+".crt"),
		filepath.Join(m.baseDir, "easy-rsa", "pki", "private", name+".key"),
		filepath.Join(m.baseDir, "easy-rsa", "pki", "reqs", name+".req"),
	}

	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}

	return nil
}

func (m *OpenVPNManager) refreshCRL() error {
	source := filepath.Join(m.baseDir, "easy-rsa", "pki", "crl.pem")
	target := filepath.Join(m.baseDir, "crl.pem")

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

func (m *OpenVPNManager) clientConfigPath(name string) string {
	return filepath.Join(m.outputDir, name+".ovpn")
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

	return os.Rename(tmpPath, dst)
}

func sanitizeName(input string) (string, error) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return "", ErrInvalidClientName
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
		return "", ErrInvalidClientName
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
