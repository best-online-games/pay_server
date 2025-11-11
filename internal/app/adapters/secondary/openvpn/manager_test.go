package openvpn

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	openvpn_domain "github.com/rostislaved/go-clean-architecture/internal/app/domain/openvpn"
)

func TestManagerEnsureCreatesAndCachesConfig(t *testing.T) {
	manager, env := newTestManager(t)
	ctx := context.Background()

	const client = "alice"

	configText, err := manager.EnsureClientConfig(ctx, client)
	if err != nil {
		t.Fatalf("ensure client: %v", err)
	}

	assertContains(t, configText, "client-common")
	assertContains(t, configText, "CERT-"+client)
	assertContains(t, configText, "KEY-"+client)

	// Config should be cached on disk and reused without extra easyrsa calls.
	assertFileContains(t, filepath.Join(env.outputDir, client+".ovpn"), configText)
	if entries := env.commandLog(); len(entries) != 1 || entries[0] != "build "+client {
		t.Fatalf("unexpected easyrsa log: %v", entries)
	}

	second, err := manager.EnsureClientConfig(ctx, client)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	if second != configText {
		t.Fatalf("expected cached config, got diff")
	}

	if entries := env.commandLog(); len(entries) != 1 {
		t.Fatalf("expected single easyrsa execution, log=%v", entries)
	}
}

func TestManagerRevokeClient(t *testing.T) {
	manager, env := newTestManager(t)
	ctx := context.Background()

	const client = "neo"

	if _, err := manager.EnsureClientConfig(ctx, client); err != nil {
		t.Fatalf("ensure client: %v", err)
	}

	if err := manager.RevokeClient(ctx, client); err != nil {
		t.Fatalf("revoke client: %v", err)
	}

	wantLog := []string{
		"build " + client,
		"revoke " + client,
		"gen-crl",
	}
	if entries := env.commandLog(); !equalSlices(entries, wantLog) {
		t.Fatalf("unexpected easyrsa log: got=%v want=%v", entries, wantLog)
	}

	indexData := env.readIndex(t)
	if !strings.Contains(indexData, "R\t0\t0\t/CN="+client) {
		t.Fatalf("expected revoked entry in index, got:\n%s", indexData)
	}

	if _, err := os.Stat(filepath.Join(env.outputDir, client+".ovpn")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected cached config removed, err=%v", err)
	}

	targetCRL := filepath.Join(env.baseDir, "crl.pem")
	targetData := readFile(t, targetCRL)
	assertContains(t, string(targetData), "CRL-")

	if err := manager.RevokeClient(ctx, client); !errors.Is(err, openvpn_domain.ErrClientAlreadyRevoked) {
		t.Fatalf("expected ErrClientAlreadyRevoked, got %v", err)
	}

	// After revocation a new ensure should recreate artifacts and append to log.
	if _, err := manager.EnsureClientConfig(ctx, client); err != nil {
		t.Fatalf("ensure after revoke: %v", err)
	}

	if entries := env.commandLog(); len(entries) != 4 {
		t.Fatalf("expected rebuild after revoke, log=%v", entries)
	}

	indexData = env.readIndex(t)
	if !strings.Contains(indexData, "V\t0\t0\t/CN="+client) {
		t.Fatalf("expected active entry in index, got:\n%s", indexData)
	}
}

// --- Test helpers ---

type testEnv struct {
	baseDir   string
	outputDir string
	logPath   string
	indexPath string
}

func newTestManager(t *testing.T) (*Manager, *testEnv) {
	t.Helper()

	temp := t.TempDir()
	baseDir := filepath.Join(temp, "server")
	outputDir := filepath.Join(temp, "clients")
	easyRSA := filepath.Join(baseDir, "easy-rsa")
	pkiDir := filepath.Join(easyRSA, "pki")

	dirs := []string{
		outputDir,
		filepath.Join(pkiDir, "issued"),
		filepath.Join(pkiDir, "private"),
		filepath.Join(pkiDir, "reqs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	writeFile(t, filepath.Join(baseDir, "client-common.txt"), "client-common\n")
	writeFile(t, filepath.Join(baseDir, "tc.key"), "-----BEGIN OpenVPN Static key-----\nFAKE\n-----END OpenVPN Static key-----\n")
	writeFile(t, filepath.Join(pkiDir, "ca.crt"), "-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----\n")
	writeFile(t, filepath.Join(pkiDir, "index.txt"), "")
	writeFile(t, filepath.Join(pkiDir, "crl.pem"), "CRL-INIT\n")

	createFakeEasyRSA(t, easyRSA)

	cfg := Config{
		BaseDir:   baseDir,
		OutputDir: outputDir,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager := New(logger, cfg)

	env := &testEnv{
		baseDir:   baseDir,
		outputDir: outputDir,
		logPath:   filepath.Join(pkiDir, "command.log"),
		indexPath: filepath.Join(pkiDir, "index.txt"),
	}

	return manager, env
}

func (e *testEnv) commandLog() []string {
	data, err := os.ReadFile(e.logPath)
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}

	return lines
}

func (e *testEnv) readIndex(t *testing.T) string {
	return string(readFile(t, e.indexPath))
}

func createFakeEasyRSA(t *testing.T, dir string) {
	t.Helper()

	script := `#!/bin/sh
set -eu

LOG_FILE="pki/command.log"
INDEX_FILE="pki/index.txt"

run_cmd() {
	printf -- "%s\n" "$1" >> "$LOG_FILE"
}

rewrite_index_with_status() {
	name="$1"
	status="$2"
	tmp="${INDEX_FILE}.tmp.$$"
	: > "$tmp"
	if [ -f "$INDEX_FILE" ]; then
		while IFS= read -r line || [ -n "$line" ]; do
			case "$line" in
				*"/CN=$name")
					printf -- "%s%s\n" "$status" "${line#?}" >> "$tmp"
					;;
				*)
					printf -- "%s\n" "$line" >> "$tmp"
					;;
			esac
		done < "$INDEX_FILE"
	fi
	if [ "$status" = "V" ]; then
		printf -- 'V\t0\t0\t/CN=%s\n' "$name" >> "$tmp"
	fi
	mv "$tmp" "$INDEX_FILE"
}

while [ $# -gt 0 ]; do
	case "$1" in
		--*) shift ;;
		*) break ;;
	esac
done

if [ $# -eq 0 ]; then
	echo "no command provided" >&2
	exit 1
fi

cmd="$1"
shift || true

case "$cmd" in
	"build-client-full")
		name="$1"
		run_cmd "build $name"
		mkdir -p pki/issued pki/private pki/reqs
		printf -- '-----BEGIN CERTIFICATE-----\nCERT-%s\n-----END CERTIFICATE-----\n' "$name" > "pki/issued/$name.crt"
		printf -- '-----BEGIN PRIVATE KEY-----\nKEY-%s\n-----END PRIVATE KEY-----\n' "$name" > "pki/private/$name.key"
		printf -- 'REQ-%s\n' "$name" > "pki/reqs/$name.req"
		rewrite_index_with_status "$name" "V"
		;;
	"revoke")
		name="$1"
		run_cmd "revoke $name"
		rewrite_index_with_status "$name" "R"
		;;
	"gen-crl")
		run_cmd "gen-crl"
		printf -- 'CRL-%s\n' "$(date +%s)" > pki/crl.pem
		;;
	*)
		echo "unknown command: $cmd" >&2
		exit 1
		;;
esac
`

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir easyrsa: %v", err)
	}

	path := filepath.Join(dir, "easyrsa")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write easyrsa: %v", err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		if t != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		} else {
			panic(err)
		}
	}

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		if t != nil {
			t.Fatalf("write %s: %v", path, err)
		} else {
			panic(err)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected %q to contain %q", haystack, needle)
	}
}

func assertFileContains(t *testing.T, path, expected string) {
	t.Helper()
	data := readFile(t, path)
	if string(data) != expected {
		t.Fatalf("expected file %s to equal cached config", path)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
