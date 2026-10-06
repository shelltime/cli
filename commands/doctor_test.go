package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// setupDoctorTest isolates HOME, installs a mock ConfigService and stubs every doctor seam that
// would touch the network, the service manager or the real daemon.
func setupDoctorTest(t *testing.T) (string, *model.MockConfigService) {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(key, "")
	}

	mc := model.NewMockConfigService(t)
	origConfig := configService
	configService = mc

	origLookPath, origDial, origLatest := doctorLookPath, doctorDialTCP, doctorFetchLatestVersion
	origResolve, origCodexStatus, origService := doctorResolveDaemonBinary, doctorCodexInstallationStatus, doctorDaemonServiceCheck
	origDaemonInstall, origCCInstall, origCodexInstall := doctorRunDaemonInstall, doctorRunCCInstall, doctorRunCodexInstall
	origWait, origStdin, origOut, origCommit := doctorDaemonStartWait, doctorStdin, doctorOut, commitID
	t.Cleanup(func() {
		configService = origConfig
		doctorLookPath, doctorDialTCP, doctorFetchLatestVersion = origLookPath, origDial, origLatest
		doctorResolveDaemonBinary, doctorCodexInstallationStatus, doctorDaemonServiceCheck = origResolve, origCodexStatus, origService
		doctorRunDaemonInstall, doctorRunCCInstall, doctorRunCodexInstall = origDaemonInstall, origCCInstall, origCodexInstall
		doctorDaemonStartWait, doctorStdin, doctorOut, commitID = origWait, origStdin, origOut, origCommit
	})

	doctorLookPath = func(name string) (string, error) {
		if name == "shelltime" {
			return "/usr/local/bin/shelltime", nil
		}
		return "", exec.ErrNotFound
	}
	doctorDialTCP = func(string, string, time.Duration) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}
	doctorFetchLatestVersion = func(context.Context) (string, error) {
		return "", errors.New("no network in tests")
	}
	doctorResolveDaemonBinary = func() (string, error) { return "/usr/local/bin/shelltime-daemon", nil }
	doctorCodexInstallationStatus = func() (bool, error) { return true, nil }
	doctorDaemonServiceCheck = func() error { return nil }
	doctorRunDaemonInstall = func(*cli.Context) error { return errors.New("daemon install is stubbed in tests") }
	doctorDaemonStartWait = 0
	doctorStdin = strings.NewReader("")
	doctorOut = io.Discard
	commitID = "v0.1.91"
	return home, mc
}

// shortSocketPath returns a unix socket path short enough for every OS (t.TempDir can exceed it).
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "stdoc")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "d.sock")
}

func doctorTestConfig(t *testing.T) model.ShellTimeConfig {
	t.Helper()
	return model.ShellTimeConfig{
		Token:       "st_v1_abcdef1234567890",
		APIEndpoint: "https://api.example.test",
		WebEndpoint: "https://web.example.test",
		DataMasking: new(true),
		SocketPath:  shortSocketPath(t),
		LogCleanup:  &model.LogCleanup{ThresholdMB: 100},
	}
}

func newTestDoctorEnv(cfg model.ShellTimeConfig) *doctorEnv {
	return &doctorEnv{
		ctx:        context.Background(),
		baseDir:    filepath.Join(os.Getenv("HOME"), model.COMMAND_BASE_STORAGE_FOLDER),
		cfg:        cfg,
		shell:      os.Getenv("SHELL"),
		socketPath: cfg.SocketPath,
	}
}

// writeDoctorConfigFile creates ~/.shelltime/config.yaml so the config-file checks pass; the
// mocked ConfigService supplies the parsed values.
func writeDoctorConfigFile(t *testing.T, home string) string {
	t.Helper()
	base := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER)
	require.NoError(t, os.MkdirAll(base, 0755))
	path := filepath.Join(base, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("token: x\n"), 0644))
	return path
}

func findDoctorResult(t *testing.T, results []doctorResult, id string) doctorResult {
	t.Helper()
	for _, r := range results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no %q result in %+v", id, results)
	return doctorResult{}
}

func hasDoctorResult(results []doctorResult, id string) bool {
	for _, r := range results {
		if r.ID == id {
			return true
		}
	}
	return false
}

func doctorTestApp() *cli.App {
	return &cli.App{Name: "t", Commands: []*cli.Command{DoctorCommand}}
}

// --- System -------------------------------------------------------------------

func TestDoctorCheckSystem(t *testing.T) {
	setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))

	results := doctorCheckSystem(env)
	assert.Equal(t, doctorInfo, findDoctorResult(t, results, "system.platform").Status)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "system.path").Status)
	assert.Equal(t, doctorSkip, findDoctorResult(t, results, "system.update").Status, "fetch error is a skip")

	doctorFetchLatestVersion = func(context.Context) (string, error) { return "v0.1.91", nil }
	assert.Equal(t, doctorOK, findDoctorResult(t, doctorCheckSystem(env), "system.update").Status)

	doctorFetchLatestVersion = func(context.Context) (string, error) { return "v0.2.0", nil }
	update := findDoctorResult(t, doctorCheckSystem(env), "system.update")
	assert.Equal(t, doctorWarn, update.Status)
	assert.Contains(t, update.Message, "v0.2.0")
	assert.Regexp(t, "shelltime update|brew upgrade", update.Fix)

	env.offline = true
	assert.Contains(t, findDoctorResult(t, doctorCheckSystem(env), "system.update").Message, "--offline")

	env.offline = false
	commitID = "dev"
	assert.Contains(t, findDoctorResult(t, doctorCheckSystem(env), "system.update").Message, "Development build")

	doctorLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	assert.Equal(t, doctorWarn, findDoctorResult(t, doctorCheckSystem(env), "system.path").Status)
}

// --- Storage ------------------------------------------------------------------

func TestDoctorCheckStorage(t *testing.T) {
	setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))

	dir := findDoctorResult(t, doctorCheckStorage(env), "storage.dir")
	assert.Equal(t, doctorFail, dir.Status)
	assert.Contains(t, dir.Fix, "shelltime init")

	require.NoError(t, os.WriteFile(env.baseDir, []byte("x"), 0644))
	dir = findDoctorResult(t, doctorCheckStorage(env), "storage.dir")
	assert.Equal(t, doctorFail, dir.Status)
	assert.Contains(t, dir.Message, "is a file")

	require.NoError(t, os.Remove(env.baseDir))
	require.NoError(t, os.MkdirAll(env.baseDir, 0755))
	results := doctorCheckStorage(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "storage.dir").Status)
	assert.False(t, hasDoctorResult(results, "storage.log"), "no log file, no log result")

	// A log over the configured threshold is flagged and the auto-fix clears it.
	env.cfg.LogCleanup = &model.LogCleanup{ThresholdMB: 1}
	logPath := filepath.Join(env.baseDir, "log.log")
	require.NoError(t, os.WriteFile(logPath, make([]byte, 2*1024*1024), 0644))
	logResult := findDoctorResult(t, doctorCheckStorage(env), "storage.log")
	assert.Equal(t, doctorWarn, logResult.Status)
	require.NotNil(t, logResult.AutoFix)
	require.NoError(t, logResult.AutoFix.Run(nil))
	assert.NoFileExists(t, logPath)

	require.NoError(t, os.WriteFile(logPath, []byte("small\n"), 0644))
	assert.Equal(t, doctorOK, findDoctorResult(t, doctorCheckStorage(env), "storage.log").Status)
}

// --- Configuration ------------------------------------------------------------

func TestDoctorCheckConfig_NoConfigFile(t *testing.T) {
	setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))
	require.NoError(t, os.MkdirAll(env.baseDir, 0755))

	results := doctorCheckConfig(env)
	require.Len(t, results, 1)
	assert.Equal(t, "config.file", results[0].ID)
	assert.Equal(t, doctorFail, results[0].Status)
}

func TestDoctorCheckConfig_Problems(t *testing.T) {
	home, _ := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	env := newTestDoctorEnv(doctorTestConfig(t))
	require.NoError(t, os.WriteFile(filepath.Join(env.baseDir, "config.toml"), []byte("Token = 'x'\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(env.baseDir, "config.local.yaml"), []byte("token: [unclosed\n"), 0644))

	env.cfg.APIEndpoint = ""
	env.cfg.Exclude = []string{"(unclosed", "^ls"}
	env.cfg.Proxy = &model.ProxyConfig{URL: "ftp://proxy:21"}
	env.cfg.EnableMetrics = new(true)

	results := doctorCheckConfig(env)
	assert.Contains(t, findDoctorResult(t, results, "config.file").Message, "config.yaml")
	shadowed := findDoctorResult(t, results, "config.shadowed")
	assert.Equal(t, doctorWarn, shadowed.Status)
	assert.Contains(t, shadowed.Message, "config.toml")
	assert.Equal(t, doctorWarn, findDoctorResult(t, results, "config.local").Status)
	assert.Equal(t, doctorFail, findDoctorResult(t, results, "config.api_endpoint").Status)
	exclude := findDoctorResult(t, results, "config.exclude")
	assert.Equal(t, doctorWarn, exclude.Status)
	assert.Contains(t, exclude.Message, "(unclosed")
	assert.NotContains(t, exclude.Message, "^ls")
	assert.Equal(t, doctorFail, findDoctorResult(t, results, "config.proxy").Status)
	assert.Equal(t, doctorWarn, findDoctorResult(t, results, "config.metrics").Status)

	env.cfg.APIEndpoint = "api.shelltime.xyz"
	assert.Equal(t, doctorFail, findDoctorResult(t, doctorCheckConfig(env), "config.api_endpoint").Status, "missing scheme")
}

func TestDoctorCheckConfig_Healthy(t *testing.T) {
	home, _ := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	env := newTestDoctorEnv(doctorTestConfig(t))
	env.cfg.Exclude = []string{"^secret"}
	env.cfg.Proxy = &model.ProxyConfig{URL: "http://user:hunter2@proxy:8080"}

	results := doctorCheckConfig(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "config.api_endpoint").Status)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "config.exclude").Status)
	proxy := findDoctorResult(t, results, "config.proxy")
	assert.Equal(t, doctorInfo, proxy.Status)
	assert.NotContains(t, proxy.Message, "hunter2")
	for _, id := range []string{"config.shadowed", "config.local", "config.parse", "config.metrics"} {
		assert.False(t, hasDoctorResult(results, id), id)
	}

	env.cfg.Proxy = nil
	t.Setenv("HTTPS_PROXY", "http://envproxy:3128")
	assert.Contains(t, findDoctorResult(t, doctorCheckConfig(env), "config.proxy").Message, "envproxy")
}

func TestDoctorChecks_ConfigParseError(t *testing.T) {
	home, _ := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	env := newTestDoctorEnv(model.ShellTimeConfig{})
	env.cfgErr = errors.New("failed to parse config file: boom")

	parse := findDoctorResult(t, doctorCheckConfig(env), "config.parse")
	assert.Equal(t, doctorFail, parse.Status)
	assert.Contains(t, parse.Message, "boom")
	assert.Equal(t, doctorSkip, findDoctorResult(t, doctorCheckAccount(env), "auth.token").Status)
	assert.Equal(t, doctorSkip, findDoctorResult(t, doctorCheckPrivacy(env), "privacy.masking").Status)
}

// --- Account ------------------------------------------------------------------

func TestDoctorCheckAccount(t *testing.T) {
	setupDoctorTest(t)

	env := newTestDoctorEnv(doctorTestConfig(t))
	env.cfg.Token = ""
	assert.Equal(t, doctorFail, findDoctorResult(t, doctorCheckAccount(env), "auth.token").Status)

	env = newTestDoctorEnv(doctorTestConfig(t))
	env.offline = true
	results := doctorCheckAccount(env)
	token := findDoctorResult(t, results, "auth.token")
	assert.Equal(t, doctorOK, token.Status)
	assert.NotContains(t, token.Message, env.cfg.Token, "token is masked")
	assert.Equal(t, doctorSkip, findDoctorResult(t, results, "auth.server").Status)

	var status int
	var body, authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		assert.Equal(t, "/api/v2/graphql", r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	env = newTestDoctorEnv(doctorTestConfig(t))
	env.cfg.APIEndpoint = srv.URL
	status, body = http.StatusOK, `{"data":{"fetchUser":{"id":1,"login":"annatar"}}}`
	server := findDoctorResult(t, doctorCheckAccount(env), "auth.server")
	assert.Equal(t, doctorOK, server.Status)
	assert.Contains(t, server.Message, "@annatar")
	assert.Equal(t, "annatar", env.login)
	assert.Equal(t, "CLI "+env.cfg.Token, authHeader)

	status, body = http.StatusUnauthorized, `{"code":401,"error":"token expired"}`
	server = findDoctorResult(t, doctorCheckAccount(env), "auth.server")
	assert.Equal(t, doctorFail, server.Status)
	assert.Contains(t, server.Fix, "shelltime auth")

	status, body = http.StatusOK, `{"data":{"fetchUser":{"id":0,"login":""}}}`
	assert.Equal(t, doctorFail, findDoctorResult(t, doctorCheckAccount(env), "auth.server").Status)

	srv.Close()
	assert.Equal(t, doctorWarn, findDoctorResult(t, doctorCheckAccount(env), "auth.server").Status, "unreachable server")
}

// --- Privacy ------------------------------------------------------------------

func TestDoctorCheckPrivacy(t *testing.T) {
	setupDoctorTest(t)

	env := newTestDoctorEnv(doctorTestConfig(t))
	env.cfg.DataMasking = new(false)
	results := doctorCheckPrivacy(env)
	assert.Equal(t, doctorWarn, findDoctorResult(t, results, "privacy.masking").Status)
	assert.Equal(t, doctorInfo, findDoctorResult(t, results, "privacy.encryption").Status)

	// Encryption without a daemon uploads plaintext.
	env = newTestDoctorEnv(doctorTestConfig(t))
	env.cfg.Encrypted = new(true)
	env.offline = true
	results = doctorCheckPrivacy(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "privacy.masking").Status)
	encryption := findDoctorResult(t, results, "privacy.encryption")
	assert.Equal(t, doctorWarn, encryption.Status)
	require.NotNil(t, encryption.AutoFix)
	assert.Equal(t, "daemon.install", encryption.AutoFix.Key)
	assert.Equal(t, doctorSkip, findDoctorResult(t, results, "privacy.encryption_key").Status)

	status, publicKey := http.StatusOK, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/opentoken/publickey", r.URL.Path)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 1, "publicKey": publicKey}})
	}))
	defer srv.Close()

	env.offline = false
	env.cfg.APIEndpoint = srv.URL
	env.daemonStatus = &daemon.StatusResponse{Version: "v0.1.91"}
	env.login = "annatar"
	results = doctorCheckPrivacy(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "privacy.encryption").Status)
	key := findDoctorResult(t, results, "privacy.encryption_key")
	assert.Equal(t, doctorWarn, key.Status, "legacy token without a key")
	assert.Contains(t, key.Fix, "https://web.example.test/users/annatar/settings/open-token")

	publicKey = "-----BEGIN PUBLIC KEY-----"
	assert.Equal(t, doctorOK, findDoctorResult(t, doctorCheckPrivacy(env), "privacy.encryption_key").Status)

	status = http.StatusInternalServerError
	assert.Equal(t, doctorSkip, findDoctorResult(t, doctorCheckPrivacy(env), "privacy.encryption_key").Status)
}

// --- Daemon -------------------------------------------------------------------

func TestDoctorCheckDaemon_NotRunning(t *testing.T) {
	setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))

	results := doctorCheckDaemon(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "daemon.binary").Status)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "daemon.service").Status)
	socket := findDoctorResult(t, results, "daemon.socket")
	assert.Equal(t, doctorWarn, socket.Status)
	require.NotNil(t, socket.AutoFix)
	assert.Equal(t, "daemon.install", socket.AutoFix.Key)

	doctorResolveDaemonBinary = func() (string, error) { return "", errors.New("not found") }
	doctorDaemonServiceCheck = func() error { return errors.New("inactive") }
	results = doctorCheckDaemon(env)
	assert.Equal(t, doctorWarn, findDoctorResult(t, results, "daemon.binary").Status)
	assert.Equal(t, doctorWarn, findDoctorResult(t, results, "daemon.service").Status)

	doctorDaemonServiceCheck = func() error { return errDoctorServiceUnsupported }
	assert.Equal(t, doctorSkip, findDoctorResult(t, doctorCheckDaemon(env), "daemon.service").Status)
}

func TestDoctorCheckDaemon_VersionMismatch(t *testing.T) {
	_, mc := setupDoctorTest(t)
	cfg := doctorTestConfig(t)
	mc.On("ReadConfigFile", mock.Anything).Return(cfg, nil)

	ln := startFakeStatusDaemon(t, cfg.SocketPath, daemon.StatusResponse{Version: "v0.1.80", Uptime: "1h"})
	defer ln.Close()
	env := newDoctorEnv(context.Background(), doctorOptions{})
	require.NotNil(t, env.daemonStatus, "doctor probes the daemon socket from the config")

	results := doctorCheckDaemon(env)
	socket := findDoctorResult(t, results, "daemon.socket")
	assert.Equal(t, doctorOK, socket.Status)
	assert.Contains(t, socket.Message, "v0.1.80")
	version := findDoctorResult(t, results, "daemon.version")
	assert.Equal(t, doctorWarn, version.Status)
	assert.Contains(t, version.Fix, "daemon reinstall")

	env.daemonStatus = &daemon.StatusResponse{Version: "0.1.91"}
	assert.False(t, hasDoctorResult(doctorCheckDaemon(env), "daemon.version"), "v-prefix is normalized")
}

// --- Shell hooks --------------------------------------------------------------

func TestDoctorCheckHooks_ShellDetection(t *testing.T) {
	setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))

	env.shell = ""
	assert.Equal(t, doctorWarn, findDoctorResult(t, doctorCheckHooks(env), "hooks.shell").Status)

	env.shell = "/bin/tcsh"
	shell := findDoctorResult(t, doctorCheckHooks(env), "hooks.shell")
	assert.Equal(t, doctorWarn, shell.Status)
	assert.Contains(t, shell.Message, "isn't supported")
}

func TestDoctorCheckHooks_ZshMissingThenFixed(t *testing.T) {
	home, _ := setupDoctorTest(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), nil, 0644))
	env := newTestDoctorEnv(doctorTestConfig(t))

	results := doctorCheckHooks(env)
	installed := findDoctorResult(t, results, "hooks.installed")
	assert.Equal(t, doctorFail, installed.Status)
	require.NotNil(t, installed.AutoFix)
	assert.Equal(t, "hooks.zsh", installed.AutoFix.Key)
	assert.Equal(t, doctorFail, findDoctorResult(t, results, "hooks.script").Status)

	require.NoError(t, installed.AutoFix.Run(nil))
	results = doctorCheckHooks(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "hooks.installed").Status)
	assert.False(t, hasDoctorResult(results, "hooks.script"))
}

func TestDoctorCheckHooks_BashNeedsPreexec(t *testing.T) {
	home, _ := setupDoctorTest(t)
	t.Setenv("SHELL", "/bin/bash")
	hooksDir := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bash.bash"), []byte("# hook\n"), 0644))
	bashrc := "# Added by shelltime CLI\n" +
		"export PATH=\"$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER + "/bin:$PATH\"\n" +
		"source " + filepath.Join(hooksDir, "bash.bash") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte(bashrc), 0644))
	env := newTestDoctorEnv(doctorTestConfig(t))

	results := doctorCheckHooks(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "hooks.installed").Status)
	assert.Equal(t, doctorFail, findDoctorResult(t, results, "hooks.bash_preexec").Status)

	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bash-preexec.sh"), []byte("# preexec\n"), 0644))
	assert.False(t, hasDoctorResult(doctorCheckHooks(env), "hooks.bash_preexec"))
}

// --- Claude Code --------------------------------------------------------------

func TestDoctorCheckClaude(t *testing.T) {
	home, _ := setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	assert.Equal(t, doctorSkip, findDoctorResult(t, doctorCheckClaude(env), "claude.detected").Status)

	require.NoError(t, os.MkdirAll(filepath.Dir(settingsPath), 0755))
	results := doctorCheckClaude(env)
	otelResult := findDoctorResult(t, results, "claude.otel")
	assert.Equal(t, doctorFail, otelResult.Status)
	require.NotNil(t, otelResult.AutoFix)
	assert.Equal(t, "claude.install", otelResult.AutoFix.Key)
	statusLine := findDoctorResult(t, results, "claude.statusline")
	assert.Equal(t, doctorInfo, statusLine.Status)
	assert.Contains(t, statusLine.Fix, doctorStatusLineCommand)
	assert.False(t, env.claudeOtel)

	require.NoError(t, os.WriteFile(settingsPath, []byte(`{"env": `), 0644))
	otelResult = findDoctorResult(t, doctorCheckClaude(env), "claude.otel")
	assert.Equal(t, doctorFail, otelResult.Status)
	assert.Nil(t, otelResult.AutoFix, "cc install can't fix invalid JSON")

	settings := `{"env": {"CLAUDE_CODE_ENABLE_TELEMETRY": "1", "OTEL_EXPORTER_OTLP_ENDPOINT": "` + model.AICodeOtelEndpoint + `"},
		"statusLine": {"type": "command", "command": "shelltime cc statusline"}}`
	require.NoError(t, os.WriteFile(settingsPath, []byte(settings), 0644))
	results = doctorCheckClaude(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "claude.otel").Status)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "claude.statusline").Status)
	assert.True(t, env.claudeOtel)
	assert.False(t, hasDoctorResult(results, "claude.legacy_env"))

	// An OTEL block written to ~/.zshrc by older versions is flagged for migration.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), nil, 0644))
	require.NoError(t, model.NewZshAICodeOtelEnvService().Install())
	legacy := findDoctorResult(t, doctorCheckClaude(env), "claude.legacy_env")
	assert.Equal(t, doctorWarn, legacy.Status)
	require.NotNil(t, legacy.AutoFix)
	assert.Equal(t, "claude.install", legacy.AutoFix.Key)

	require.NoError(t, os.WriteFile(settingsPath, []byte(`{"statusLine": {"command": "~/bin/my-statusline"}}`), 0644))
	statusLine = findDoctorResult(t, doctorCheckClaude(env), "claude.statusline")
	assert.Equal(t, doctorInfo, statusLine.Status)
	assert.Contains(t, statusLine.Message, "my-statusline")
}

// --- Codex --------------------------------------------------------------------

func TestDoctorCheckCodex(t *testing.T) {
	home, _ := setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))
	configPath := filepath.Join(home, ".codex", "config.toml")

	assert.Equal(t, doctorSkip, findDoctorResult(t, doctorCheckCodex(env), "codex.detected").Status)

	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	otelResult := findDoctorResult(t, doctorCheckCodex(env), "codex.otel")
	assert.Equal(t, doctorFail, otelResult.Status)
	require.NotNil(t, otelResult.AutoFix)
	assert.Equal(t, "codex.install", otelResult.AutoFix.Key)

	require.NoError(t, os.WriteFile(configPath, []byte("[otel.exporter.otlp-grpc]\nendpoint = \"http://localhost:4317\"\n"), 0644))
	otelResult = findDoctorResult(t, doctorCheckCodex(env), "codex.otel")
	assert.Equal(t, doctorFail, otelResult.Status)
	assert.Contains(t, otelResult.Message, "4317")
	assert.NotNil(t, otelResult.AutoFix)

	require.NoError(t, os.WriteFile(configPath, []byte("[otel\n"), 0644))
	otelResult = findDoctorResult(t, doctorCheckCodex(env), "codex.otel")
	assert.Equal(t, doctorFail, otelResult.Status)
	assert.Nil(t, otelResult.AutoFix, "codex install can't fix invalid TOML")

	require.NoError(t, os.Remove(configPath))
	require.NoError(t, model.NewCodexOtelConfigService().Install())
	results := doctorCheckCodex(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "codex.otel").Status)
	assert.True(t, env.codexOtel)
	assert.False(t, hasDoctorResult(results, "codex.auth"))

	doctorCodexInstallationStatus = daemon.CodexInstallationStatus
	assert.Equal(t, doctorInfo, findDoctorResult(t, doctorCheckCodex(env), "codex.auth").Status, "no auth.json")
}

// --- AI usage receiver --------------------------------------------------------

func TestDoctorCheckOtelReceiver(t *testing.T) {
	setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))

	assert.Equal(t, doctorSkip, findDoctorResult(t, doctorCheckOtelReceiver(env), "otel.enabled").Status)

	env.claudeOtel = true
	enabled := findDoctorResult(t, doctorCheckOtelReceiver(env), "otel.enabled")
	assert.Equal(t, doctorFail, enabled.Status)
	assert.Contains(t, enabled.Fix, "aiCodeOtel.enabled: true")

	env.cfg.AICodeOtel = &model.AICodeOtel{Enabled: new(true), GRPCPort: 4317}
	assert.Equal(t, doctorFail, findDoctorResult(t, doctorCheckOtelReceiver(env), "otel.port").Status)

	env.cfg.AICodeOtel.GRPCPort = 0
	assert.Equal(t, doctorSkip, findDoctorResult(t, doctorCheckOtelReceiver(env), "otel.listening").Status, "daemon not running")

	env.daemonStatus = &daemon.StatusResponse{Version: "v0.1.91"}
	listening := findDoctorResult(t, doctorCheckOtelReceiver(env), "otel.listening")
	assert.Equal(t, doctorFail, listening.Status)
	require.NotNil(t, listening.AutoFix)
	assert.Equal(t, "daemon.install", listening.AutoFix.Key)

	var dialed string
	doctorDialTCP = func(_, address string, _ time.Duration) (net.Conn, error) {
		dialed = address
		client, server := net.Pipe()
		server.Close()
		return client, nil
	}
	assert.Equal(t, doctorOK, findDoctorResult(t, doctorCheckOtelReceiver(env), "otel.listening").Status)
	assert.Equal(t, "127.0.0.1:54027", dialed)
}

// --- Sync ---------------------------------------------------------------------

func TestDoctorCheckSync(t *testing.T) {
	home, _ := setupDoctorTest(t)
	env := newTestDoctorEnv(doctorTestConfig(t))

	results := doctorCheckSync(env)
	assert.Equal(t, doctorOK, findDoctorResult(t, results, "sync.pending").Status)
	assert.False(t, hasDoctorResult(results, "sync.heartbeats"))

	base := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER)
	require.NoError(t, os.MkdirAll(base, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "sync-pending.jsonl"), []byte("{}\n\n  \n{}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "coding-heartbeat.data.log"), []byte("a\nb\nc"), 0644))

	results = doctorCheckSync(env)
	pending := findDoctorResult(t, results, "sync.pending")
	assert.Equal(t, doctorWarn, pending.Status)
	assert.Contains(t, pending.Message, "2 upload")
	assert.Contains(t, findDoctorResult(t, results, "sync.heartbeats").Message, "3 editor")
}

func TestCountNonEmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")

	_, err := countNonEmptyLines(path)
	assert.True(t, os.IsNotExist(err))

	// A line longer than bufio.Scanner's limit, CRLF endings, a blank line and no final newline.
	content := strings.Repeat("x", 200*1024) + "\r\n\r\n \t\nsecond\r\nthird"
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	count, err := countNonEmptyLines(path)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

// --- Rendering ----------------------------------------------------------------

func TestGroupDoctorActionsAndFixes(t *testing.T) {
	daemonFix := &doctorFix{Key: "daemon.install"}
	hookFix := &doctorFix{Key: "hooks.zsh"}
	results := []doctorResult{
		{ID: "a", Status: doctorWarn, Fix: "Run `shelltime daemon install` (downloads it).", AutoFix: daemonFix},
		{ID: "b", Status: doctorOK},
		{ID: "c", Status: doctorFail, Fix: "Run `shelltime hooks install`.", AutoFix: hookFix},
		{ID: "d", Status: doctorWarn, Fix: "Run `shelltime daemon install`.", AutoFix: daemonFix},
		{ID: "e", Status: doctorFail, Fix: "Run `shelltime init`."},
		{ID: "f", Status: doctorFail, Fix: "Run `shelltime init`."},
		{ID: "g", Status: doctorWarn},
		{ID: "h", Status: doctorInfo, Fix: "tip", AutoFix: &doctorFix{Key: "info.only"}},
	}

	actions := groupDoctorActions(results)
	require.Len(t, actions, 4)
	assert.Equal(t, []string{"c"}, doctorActionIDs(actions[0]), "failures come first")
	assert.Equal(t, []string{"e", "f"}, doctorActionIDs(actions[1]), "same fix text is grouped")
	assert.Equal(t, []string{"a", "d"}, doctorActionIDs(actions[2]), "same auto-fix key is grouped")
	assert.Equal(t, []string{"g"}, doctorActionIDs(actions[3]))

	fixes := collectDoctorFixes(results)
	require.Len(t, fixes, 2, "info results never contribute fixes")
	assert.Same(t, hookFix, fixes[0])
	assert.Same(t, daemonFix, fixes[1])

	assert.NotPanics(t, func() {
		printDoctorReport(results)
		printDoctorSummary(nil, false)
	})
}

func doctorActionIDs(action *doctorAction) []string {
	var ids []string
	for _, r := range action.problems {
		ids = append(ids, r.ID)
	}
	return ids
}

// --- End to end ---------------------------------------------------------------

func TestCommandDoctor_HealthySetupExitsCleanly(t *testing.T) {
	home, mc := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	mc.On("ReadConfigFile", mock.Anything).Return(doctorTestConfig(t), nil)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), nil, 0644))
	require.NoError(t, model.NewZshHookService().Install())

	// The daemon isn't running, but that is only a warning.
	require.NoError(t, doctorTestApp().Run([]string{"t", "doctor", "--offline"}))
}

func TestCommandDoctor_JSONReport(t *testing.T) {
	home, mc := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	mc.On("ReadConfigFile", mock.Anything).Return(doctorTestConfig(t), nil)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), nil, 0644))
	out := &bytes.Buffer{}
	doctorOut = out

	err := doctorTestApp().Run([]string{"t", "doctor", "--offline", "--format", "json"})
	require.ErrorIs(t, err, ErrDoctorFoundProblems)

	var report doctorReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report), out.String())
	assert.Equal(t, "v0.1.91", report.Version)
	assert.Equal(t, 2, report.Summary.Fail, "hook missing and hook script missing")
	installed := findDoctorResult(t, report.Checks, "hooks.installed")
	assert.Equal(t, "Shell Hooks", installed.Section)
	require.NotNil(t, installed.AutoFix)
	assert.Equal(t, "hooks.zsh", installed.AutoFix.Key)
	assert.NotContains(t, out.String(), home, "paths under HOME are shown as ~")
}

func TestCommandDoctor_ConfigErrorKeepsChecking(t *testing.T) {
	home, mc := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	out := &bytes.Buffer{}
	doctorOut = out

	err := doctorTestApp().Run([]string{"t", "doctor", "--offline", "--format", "json"})
	require.ErrorIs(t, err, ErrDoctorFoundProblems)

	var report doctorReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, doctorFail, findDoctorResult(t, report.Checks, "config.parse").Status)
	assert.True(t, hasDoctorResult(report.Checks, "hooks.installed"), "checks after the config still run")
}

func TestCommandDoctor_FlagValidation(t *testing.T) {
	setupDoctorTest(t)

	err := doctorTestApp().Run([]string{"t", "doctor", "--format", "xml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")

	err = doctorTestApp().Run([]string{"t", "doctor", "--fix", "--format", "json"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--fix")
}

func TestCommandDoctor_FixAppliesAutomaticFixes(t *testing.T) {
	home, mc := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	cfg := doctorTestConfig(t)
	cfg.AICodeOtel = &model.AICodeOtel{Enabled: new(true)}
	mc.On("ReadConfigFile", mock.Anything).Return(cfg, nil)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), nil, 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0755))

	daemonInstalls := 0
	doctorRunDaemonInstall = func(*cli.Context) error {
		daemonInstalls++
		return nil
	}
	doctorDaemonServiceCheck = func() error { return errors.New("inactive") }

	// Problems left after the fixes are only daemon warnings, so doctor exits cleanly.
	require.NoError(t, doctorTestApp().Run([]string{"t", "doctor", "--offline", "--fix", "--yes"}))

	assert.Equal(t, 1, daemonInstalls, "one daemon install for every daemon problem")
	zshrc, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	require.NoError(t, err)
	assert.Contains(t, string(zshrc), "# Added by shelltime CLI")
	settings, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(settings), model.AICodeOtelEndpoint)
}

func TestCommandDoctor_FixDeclined(t *testing.T) {
	home, mc := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	mc.On("ReadConfigFile", mock.Anything).Return(doctorTestConfig(t), nil)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), nil, 0644))
	doctorStdin = strings.NewReader("n\n")

	err := doctorTestApp().Run([]string{"t", "doctor", "--offline", "--fix"})
	require.ErrorIs(t, err, ErrDoctorFoundProblems)

	zshrc, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	require.NoError(t, err)
	assert.Empty(t, zshrc, "nothing is changed when the prompt is declined")
}
