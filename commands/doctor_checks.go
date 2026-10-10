package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

const (
	doctorDaemonProbeTimeout = 2 * time.Second
	doctorNetworkTimeout     = 8 * time.Second
	doctorUpdateTimeout      = 3 * time.Second
	doctorStatusLineCommand  = "shelltime cc statusline"
)

var errDoctorServiceUnsupported = errors.New("daemon service is not supported on this OS")

// Seams so tests can stub out the parts of doctor that touch the system or the network.
var (
	doctorLookPath                = exec.LookPath
	doctorDialTCP                 = net.DialTimeout
	doctorFetchLatestVersion      = model.FetchLatestVersion
	doctorResolveDaemonBinary     = model.ResolveDaemonBinaryPath
	doctorCodexInstallationStatus = daemon.CodexInstallationStatus
	doctorDaemonServiceCheck      = func() error {
		installer, err := model.NewDaemonInstaller("", "", "")
		if err != nil {
			return errDoctorServiceUnsupported
		}
		return installer.Check()
	}
	doctorRunDaemonInstall = commandDaemonInstall
	doctorRunCCInstall     = commandCCInstall
	doctorRunCodexInstall  = commandCodexInstall

	// doctorDaemonStartWait bounds how long the daemon fix waits for the new daemon's socket.
	doctorDaemonStartWait = 5 * time.Second

	doctorStdin io.Reader = os.Stdin
	doctorOut   io.Writer = os.Stdout
)

type doctorOptions struct {
	offline bool
}

// doctorEnv is gathered once per run and shared by every section. Sections run in order, and some
// record what they found (login, which AI tools report usage) for the sections after them.
type doctorEnv struct {
	ctx        context.Context
	offline    bool
	baseDir    string
	configPath string
	cfg        model.ShellTimeConfig
	cfgErr     error
	shell      string
	socketPath string

	daemonStatus *daemon.StatusResponse

	login      string
	claudeOtel bool
	codexOtel  bool
}

type doctorSection struct {
	title string
	run   func(env *doctorEnv) []doctorResult
}

func doctorSections() []doctorSection {
	return []doctorSection{
		{"System", doctorCheckSystem},
		{"Storage", doctorCheckStorage},
		{"Configuration", doctorCheckConfig},
		{"Account", doctorCheckAccount},
		{"Privacy", doctorCheckPrivacy},
		{"Daemon", doctorCheckDaemon},
		{"Shell Hooks", doctorCheckHooks},
		{"Claude Code", doctorCheckClaude},
		{"Codex", doctorCheckCodex},
		{"AI Usage Receiver", doctorCheckOtelReceiver},
		{"Sync", doctorCheckSync},
	}
}

func runDoctorChecks(ctx context.Context, opts doctorOptions) []doctorResult {
	env := newDoctorEnv(ctx, opts)
	// Show paths under the home directory as ~/... to keep lines short and reports shareable.
	home, _ := os.UserHomeDir()
	tilde := func(s string) string {
		if home == "" || home == "/" {
			return s
		}
		return strings.ReplaceAll(s, home+string(filepath.Separator), "~"+string(filepath.Separator))
	}

	var results []doctorResult
	for _, section := range doctorSections() {
		for _, r := range section.run(env) {
			r.Section = section.title
			r.Message = tilde(r.Message)
			r.Fix = tilde(r.Fix)
			results = append(results, r)
		}
	}
	return results
}

func newDoctorEnv(ctx context.Context, opts doctorOptions) *doctorEnv {
	if ctx == nil {
		ctx = context.Background()
	}
	env := &doctorEnv{
		ctx:        ctx,
		offline:    opts.offline,
		baseDir:    os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER),
		shell:      os.Getenv("SHELL"),
		socketPath: model.DefaultSocketPath,
	}
	env.configPath, _ = model.ConfigFilePaths(env.baseDir)
	env.cfg, env.cfgErr = configService.ReadConfigFile(ctx)
	if env.cfgErr == nil && env.cfg.SocketPath != "" {
		env.socketPath = env.cfg.SocketPath
	}
	if status, _, err := requestDaemonStatus(env.socketPath, doctorDaemonProbeTimeout); err == nil {
		env.daemonStatus = status
	}
	return env
}

// configFileHint names the config file to edit in fix messages.
func (env *doctorEnv) configFileHint() string {
	if env.configPath != "" {
		return env.configPath
	}
	return filepath.Join(env.baseDir, "config.yaml")
}

func (env *doctorEnv) skipWithoutConfig(id string) []doctorResult {
	return []doctorResult{{ID: id, Status: doctorSkip, Message: "Skipped: the config couldn't be loaded (see Configuration)."}}
}

func doctorDaemonInstallFix(env *doctorEnv) *doctorFix {
	return &doctorFix{
		Key:   "daemon.install",
		Label: "Install and start the daemon service (shelltime daemon install)",
		Run: func(c *cli.Context) error {
			if err := doctorRunDaemonInstall(c); err != nil {
				return err
			}
			// Give the freshly started daemon a moment to open its socket before re-checking.
			for deadline := time.Now().Add(doctorDaemonStartWait); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
				if _, _, err := requestDaemonStatus(env.socketPath, 500*time.Millisecond); err == nil {
					break
				}
			}
			return nil
		},
	}
}

// --- System -------------------------------------------------------------------

func doctorCheckSystem(env *doctorEnv) []doctorResult {
	version := doctorVersion()
	results := []doctorResult{{
		ID:      "system.platform",
		Status:  doctorInfo,
		Message: fmt.Sprintf("shelltime %s on %s/%s", version, runtime.GOOS, runtime.GOARCH),
	}}

	if path, err := doctorLookPath("shelltime"); err == nil {
		results = append(results, doctorResult{ID: "system.path", Status: doctorOK, Message: fmt.Sprintf("shelltime is on your PATH (%s).", path)})
	} else {
		results = append(results, doctorResult{
			ID:      "system.path",
			Status:  doctorWarn,
			Message: "The shelltime command isn't on your PATH, so the shell hooks can't run it.",
			Fix:     "Open a new terminal. If it's still missing, run `shelltime hooks install` (it adds ~/.shelltime/bin to your PATH).",
		})
	}

	update := doctorResult{ID: "system.update"}
	switch {
	case env.offline:
		update.Status, update.Message = doctorSkip, "Skipped the update check (--offline)."
	case version == "dev":
		update.Status, update.Message = doctorSkip, "Development build; skipped the update check."
	default:
		ctx, cancel := context.WithTimeout(env.ctx, doctorUpdateTimeout)
		latest, err := doctorFetchLatestVersion(ctx)
		cancel()
		cmp := compareToLatest(version, latest)
		switch {
		case err != nil:
			update.Status, update.Message = doctorSkip, fmt.Sprintf("Couldn't check for updates: %v", err)
		case cmp == 0:
			update.Status, update.Message = doctorOK, fmt.Sprintf("You're on the latest version (%s).", latest)
		case cmp > 0:
			update.Status, update.Message = doctorOK, fmt.Sprintf("You're ahead of the latest release (%s, you have %s).", latest, version)
		default:
			update.Status = doctorWarn
			update.Message = fmt.Sprintf("A newer version is available: %s (you have %s).", latest, version)
			update.Fix = doctorUpdateHint()
		}
	}
	return append(results, update)
}

func doctorUpdateHint() string {
	if cliPath, err := model.ResolveCLIBinaryPath(); err == nil && model.DetectInstallKind(cliPath) == model.InstallKindHomebrew {
		return fmt.Sprintf("Run `%s`.", model.HomebrewUpgradeCommand(cliPath))
	}
	return "Run `shelltime update`."
}

// --- Storage ------------------------------------------------------------------

func doctorCheckStorage(env *doctorEnv) []doctorResult {
	dir := env.baseDir
	result := doctorResult{ID: "storage.dir"}
	info, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		result.Status = doctorFail
		result.Message = fmt.Sprintf("%s doesn't exist, so shelltime has nowhere to keep its config or buffer commands.", dir)
		result.Fix = "Run `shelltime init` to sign in and set everything up."
		return []doctorResult{result}
	case err != nil:
		result.Status = doctorFail
		result.Message = fmt.Sprintf("Can't access %s: %v", dir, err)
		result.Fix = fmt.Sprintf("Make sure %s is owned by you and readable.", dir)
		return []doctorResult{result}
	case !info.IsDir():
		result.Status = doctorFail
		result.Message = fmt.Sprintf("%s is a file, but shelltime needs it to be a directory.", dir)
		result.Fix = fmt.Sprintf("Move the file away (`mv %s %s.bak`), then run `shelltime init`.", dir, dir)
		return []doctorResult{result}
	}

	if err := checkDirWritable(dir); err != nil {
		result.Status = doctorFail
		result.Message = fmt.Sprintf("%s isn't writable, so commands can't be recorded: %v", dir, err)
		result.Fix = fmt.Sprintf("Take ownership of it: `sudo chown -R $(whoami) %s`.", dir)
	} else {
		result.Status = doctorOK
		result.Message = fmt.Sprintf("%s exists and is writable.", dir)
	}
	results := []doctorResult{result}

	thresholdMB := int64(100)
	if env.cfg.LogCleanup != nil && env.cfg.LogCleanup.ThresholdMB > 0 {
		thresholdMB = env.cfg.LogCleanup.ThresholdMB
	}
	logPath := filepath.Join(dir, "log.log")
	if logInfo, err := os.Stat(logPath); err == nil && !logInfo.IsDir() {
		sizeMB := logInfo.Size() / (1024 * 1024)
		if logInfo.Size() > thresholdMB*1024*1024 {
			results = append(results, doctorResult{
				ID:      "storage.log",
				Status:  doctorWarn,
				Message: fmt.Sprintf("%s is %dMB, over the %dMB cleanup threshold.", logPath, sizeMB, thresholdMB),
				Fix:     "Run `shelltime gc` to clear it.",
				AutoFix: &doctorFix{
					Key:   "storage.log",
					Label: fmt.Sprintf("Delete the oversized %s (a fresh one is started)", logPath),
					Run: func(*cli.Context) error {
						_, err := model.CleanLogFile(logPath, thresholdMB*1024*1024, false)
						return err
					},
				},
			})
		} else {
			results = append(results, doctorResult{ID: "storage.log", Status: doctorOK, Message: fmt.Sprintf("Log file size is fine (%dMB).", sizeMB)})
		}
	}
	return results
}

func checkDirWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// --- Configuration ------------------------------------------------------------

func doctorCheckConfig(env *doctorEnv) []doctorResult {
	base, local := model.ConfigFilePaths(env.baseDir)
	if base == "" {
		return []doctorResult{{
			ID:      "config.file",
			Status:  doctorFail,
			Message: fmt.Sprintf("No config file in %s, so nothing can be tracked or uploaded.", env.baseDir),
			Fix:     "Run `shelltime init` to sign in and set everything up.",
		}}
	}
	results := []doctorResult{{ID: "config.file", Status: doctorOK, Message: fmt.Sprintf("Using %s.", base)}}

	for _, name := range []string{"config.yaml", "config.yml", "config.toml"} {
		other := filepath.Join(env.baseDir, name)
		if other == base {
			continue
		}
		if _, err := os.Stat(other); err == nil {
			results = append(results, doctorResult{
				ID:      "config.shadowed",
				Status:  doctorWarn,
				Message: fmt.Sprintf("%s is ignored because %s takes precedence.", other, filepath.Base(base)),
				Fix:     fmt.Sprintf("Move any settings you still need into %s, then delete %s.", base, other),
			})
		}
	}

	if env.cfgErr != nil {
		return append(results, doctorResult{
			ID:      "config.parse",
			Status:  doctorFail,
			Message: fmt.Sprintf("Your config couldn't be loaded, so shelltime can't track or upload anything: %v", env.cfgErr),
			Fix:     fmt.Sprintf("Fix the syntax error in %s. Every option is documented in docs/CONFIG.md.", base),
		})
	}

	if local != "" {
		if err := model.ValidateConfigFile(local); err != nil {
			results = append(results, doctorResult{
				ID:      "config.local",
				Status:  doctorWarn,
				Message: fmt.Sprintf("%s can't be parsed, so all of its overrides are silently ignored: %v", local, err),
				Fix:     fmt.Sprintf("Fix the syntax error in %s.", local),
			})
		} else {
			results = append(results, doctorResult{ID: "config.local", Status: doctorInfo, Message: fmt.Sprintf("Overrides from %s are applied.", local)})
		}
	}

	cfg := env.cfg
	endpointFix := fmt.Sprintf("Set `apiEndpoint: https://api.shelltime.xyz` in %s.", base)
	if cfg.APIEndpoint == "" {
		results = append(results, doctorResult{
			ID:      "config.api_endpoint",
			Status:  doctorFail,
			Message: "apiEndpoint isn't set, so nothing can be uploaded.",
			Fix:     endpointFix,
		})
	} else if u, err := url.Parse(cfg.APIEndpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		results = append(results, doctorResult{
			ID:      "config.api_endpoint",
			Status:  doctorFail,
			Message: fmt.Sprintf("apiEndpoint %q isn't a valid http(s) URL, so nothing can be uploaded.", cfg.APIEndpoint),
			Fix:     endpointFix,
		})
	} else {
		results = append(results, doctorResult{ID: "config.api_endpoint", Status: doctorOK, Message: fmt.Sprintf("API endpoint: %s", cfg.APIEndpoint)})
	}

	var badPatterns []string
	for _, pattern := range cfg.Exclude {
		if pattern == "" {
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			badPatterns = append(badPatterns, fmt.Sprintf("%q", pattern))
		}
	}
	if len(badPatterns) > 0 {
		results = append(results, doctorResult{
			ID:      "config.exclude",
			Status:  doctorWarn,
			Message: fmt.Sprintf("These exclude patterns aren't valid regular expressions and are skipped, so the commands they should hide are still uploaded: %s", strings.Join(badPatterns, ", ")),
			Fix:     fmt.Sprintf("Fix or remove them under `exclude` in %s.", base),
		})
	} else if len(cfg.Exclude) > 0 {
		results = append(results, doctorResult{ID: "config.exclude", Status: doctorOK, Message: fmt.Sprintf("%d exclude pattern(s), all valid.", len(cfg.Exclude))})
	}

	if cfg.Proxy != nil && strings.TrimSpace(cfg.Proxy.URL) != "" {
		if _, err := model.ParseProxyURL(cfg.Proxy.URL); err != nil {
			results = append(results, doctorResult{
				ID:      "config.proxy",
				Status:  doctorFail,
				Message: fmt.Sprintf("proxy.url is invalid, so shelltime ignores it and falls back to the HTTP(S)_PROXY environment variables: %v", err),
				Fix:     fmt.Sprintf("Set proxy.url in %s to an http://, https://, socks5:// or socks5h:// URL.", base),
			})
		} else {
			results = append(results, doctorResult{ID: "config.proxy", Status: doctorInfo, Message: fmt.Sprintf("Requests go through proxy %s.", model.RedactProxyURL(cfg.Proxy.URL))})
		}
	} else if proxy := doctorEnvProxy(); proxy != "" {
		results = append(results, doctorResult{ID: "config.proxy", Status: doctorInfo, Message: fmt.Sprintf("Using the proxy from your environment (%s).", model.RedactProxyURL(proxy))})
	}

	if cfg.EnableMetrics != nil && *cfg.EnableMetrics {
		results = append(results, doctorResult{
			ID:      "config.metrics",
			Status:  doctorWarn,
			Message: "enableMetrics is on, which adds overhead to every tracked command.",
			Fix:     fmt.Sprintf("Set `enableMetrics: false` in %s unless the shelltime team asked you to turn it on.", base),
		})
	}
	return results
}

func doctorEnvProxy() string {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// --- Account ------------------------------------------------------------------

func doctorCheckAccount(env *doctorEnv) []doctorResult {
	if env.cfgErr != nil {
		return env.skipWithoutConfig("auth.token")
	}
	cfg := env.cfg
	if strings.TrimSpace(cfg.Token) == "" {
		return []doctorResult{{
			ID:      "auth.token",
			Status:  doctorFail,
			Message: "No API token is configured, so your commands and AI usage can't be uploaded.",
			Fix:     "Run `shelltime auth` to sign in.",
		}}
	}
	results := []doctorResult{{ID: "auth.token", Status: doctorOK, Message: fmt.Sprintf("An API token is configured (%s).", maskDoctorToken(cfg.Token))}}

	switch {
	case env.offline:
		return append(results, doctorResult{ID: "auth.server", Status: doctorSkip, Message: "Skipped checking the token with the server (--offline)."})
	case cfg.APIEndpoint == "":
		return append(results, doctorResult{ID: "auth.server", Status: doctorSkip, Message: "Skipped checking the token: apiEndpoint isn't set."})
	}

	ctx, cancel := context.WithTimeout(env.ctx, doctorNetworkTimeout)
	defer cancel()
	profile, err := model.FetchCurrentUserProfile(ctx, cfg)

	result := doctorResult{ID: "auth.server"}
	var statusErr *model.HTTPStatusError
	switch {
	case err == nil && profile.FetchUser.Login != "":
		env.login = profile.FetchUser.Login
		result.Status = doctorOK
		result.Message = fmt.Sprintf("Signed in as @%s on %s.", env.login, cfg.APIEndpoint)
	case errors.As(err, &statusErr) && (statusErr.StatusCode == 401 || statusErr.StatusCode == 403):
		result.Status = doctorFail
		result.Message = fmt.Sprintf("The server rejected your token (HTTP %d): it's expired, revoked or disabled, so nothing is being uploaded.", statusErr.StatusCode)
		result.Fix = "Run `shelltime auth` to sign in again."
	case err == nil:
		result.Status = doctorFail
		result.Message = "The server didn't recognise your token, so nothing is being uploaded."
		result.Fix = "Run `shelltime auth` to sign in again."
	default:
		result.Status = doctorWarn
		result.Message = fmt.Sprintf("Couldn't reach %s to verify your token: %v", cfg.APIEndpoint, err)
		result.Fix = "Check your network connection and proxy settings, then run `shelltime doctor` again."
	}
	return append(results, result)
}

func maskDoctorToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "…" + token[len(token)-4:]
}

// --- Privacy ------------------------------------------------------------------

func doctorCheckPrivacy(env *doctorEnv) []doctorResult {
	if env.cfgErr != nil {
		return env.skipWithoutConfig("privacy.masking")
	}
	cfg := env.cfg
	configFile := env.configFileHint()

	var results []doctorResult
	if cfg.DataMasking != nil && !*cfg.DataMasking {
		results = append(results, doctorResult{
			ID:      "privacy.masking",
			Status:  doctorWarn,
			Message: "Data masking is off: commands are uploaded exactly as typed, including any tokens in them.",
			Fix:     fmt.Sprintf("Set `dataMasking: true` in %s.", configFile),
		})
	} else {
		results = append(results, doctorResult{
			ID:      "privacy.masking",
			Status:  doctorOK,
			Message: "Data masking is on: JWT-style tokens in commands are masked before upload.",
			Fix:     "To hide other secrets, add regular expressions under `exclude` in your config.",
		})
	}

	if cfg.Encrypted == nil || !*cfg.Encrypted {
		return append(results, doctorResult{
			ID:      "privacy.encryption",
			Status:  doctorInfo,
			Message: "End-to-end encryption is off; uploads are protected by TLS only.",
			Fix:     fmt.Sprintf("Set `encrypted: true` in %s to encrypt commands before upload (needs the daemon).", configFile),
		})
	}

	if env.daemonStatus == nil {
		results = append(results, doctorResult{
			ID:      "privacy.encryption",
			Status:  doctorWarn,
			Message: "Encryption is on, but only the daemon encrypts and it isn't running, so commands are uploaded unencrypted.",
			Fix:     "Run `shelltime daemon install`.",
			AutoFix: doctorDaemonInstallFix(env),
		})
	} else {
		results = append(results, doctorResult{ID: "privacy.encryption", Status: doctorOK, Message: "Encryption is on and the daemon is running to encrypt uploads."})
	}

	switch {
	case env.offline:
		return append(results, doctorResult{ID: "privacy.encryption_key", Status: doctorSkip, Message: "Skipped checking your token's encryption key (--offline)."})
	case strings.TrimSpace(cfg.Token) == "" || cfg.APIEndpoint == "":
		return results
	}

	ctx, cancel := context.WithTimeout(env.ctx, doctorNetworkTimeout)
	defer cancel()
	key, err := model.GetOpenTokenPublicKey(ctx, model.Endpoint{Token: cfg.Token, APIEndpoint: cfg.APIEndpoint}, 0)
	switch {
	case err != nil:
		results = append(results, doctorResult{ID: "privacy.encryption_key", Status: doctorSkip, Message: fmt.Sprintf("Couldn't check your token's encryption key: %v", err)})
	case strings.TrimSpace(key.PublicKey) == "":
		tokensPage := strings.TrimRight(cfg.WebEndpoint, "/") + " (Settings → Open Token)"
		if env.login != "" {
			tokensPage = fmt.Sprintf("%s/users/%s/settings/open-token", strings.TrimRight(cfg.WebEndpoint, "/"), env.login)
		}
		results = append(results, doctorResult{
			ID:      "privacy.encryption_key",
			Status:  doctorWarn,
			Message: "Encryption is on, but your token is an older one without an encryption key, so commands are uploaded unencrypted.",
			Fix:     fmt.Sprintf("Create a new token at %s, then run `shelltime auth --token <new token>`.", tokensPage),
		})
	default:
		results = append(results, doctorResult{ID: "privacy.encryption_key", Status: doctorOK, Message: "Your token has an encryption key."})
	}
	return results
}

// --- Daemon -------------------------------------------------------------------

func doctorCheckDaemon(env *doctorEnv) []doctorResult {
	installFix := doctorDaemonInstallFix(env)
	var results []doctorResult

	if path, err := doctorResolveDaemonBinary(); err == nil {
		results = append(results, doctorResult{ID: "daemon.binary", Status: doctorOK, Message: fmt.Sprintf("Daemon binary: %s", path)})
	} else {
		results = append(results, doctorResult{
			ID:      "daemon.binary",
			Status:  doctorWarn,
			Message: "The shelltime-daemon binary wasn't found.",
			Fix:     "Run `shelltime daemon install` (it downloads the daemon if needed).",
			AutoFix: installFix,
		})
	}

	switch err := doctorDaemonServiceCheck(); {
	case errors.Is(err, errDoctorServiceUnsupported):
		results = append(results, doctorResult{ID: "daemon.service", Status: doctorSkip, Message: fmt.Sprintf("The daemon service isn't supported on %s.", runtime.GOOS)})
	case err != nil:
		results = append(results, doctorResult{
			ID:      "daemon.service",
			Status:  doctorWarn,
			Message: "The daemon service isn't installed or isn't running.",
			Fix:     "Run `shelltime daemon install`.",
			AutoFix: installFix,
		})
	default:
		results = append(results, doctorResult{ID: "daemon.service", Status: doctorOK, Message: "The daemon service is registered with the system service manager."})
	}

	status := env.daemonStatus
	if status == nil {
		return append(results, doctorResult{
			ID:      "daemon.socket",
			Status:  doctorWarn,
			Message: fmt.Sprintf("The daemon isn't responding on %s, so syncing falls back to the slower direct path, and encryption and AI coding usage tracking don't work.", env.socketPath),
			Fix:     "Run `shelltime daemon install`.",
			AutoFix: installFix,
		})
	}
	results = append(results, doctorResult{
		ID:      "daemon.socket",
		Status:  doctorOK,
		Message: fmt.Sprintf("The daemon is responding on %s (%s, up %s).", env.socketPath, status.Version, status.Uptime),
	})

	cliVersion := model.NormalizeVersion(doctorVersion())
	daemonVersion := model.NormalizeVersion(status.Version)
	if cliVersion != "dev" && daemonVersion != "" && daemonVersion != "dev" && cliVersion != daemonVersion {
		results = append(results, doctorResult{
			ID:      "daemon.version",
			Status:  doctorWarn,
			Message: fmt.Sprintf("The daemon is running %s but the CLI is %s, so the daemon is still on the old binary.", status.Version, doctorVersion()),
			Fix:     "Run `shelltime daemon reinstall`.",
			AutoFix: installFix,
		})
	}
	return results
}

// --- Shell hooks --------------------------------------------------------------

var doctorShellFiles = map[string]struct{ rc, hook string }{
	"zsh":  {"~/.zshrc", "zsh.zsh"},
	"bash": {"~/.bashrc", "bash.bash"},
	"fish": {"~/.config/fish/config.fish", "fish.fish"},
}

func doctorCheckHooks(env *doctorEnv) []doctorResult {
	if env.shell == "" {
		return []doctorResult{{
			ID:      "hooks.shell",
			Status:  doctorWarn,
			Message: "$SHELL isn't set, so doctor can't tell which shell hook to check.",
			Fix:     "Run `shelltime doctor` from your usual terminal, or run `shelltime hooks install` to set up zsh, bash and fish.",
		}}
	}

	var svc model.ShellHookService
	for _, candidate := range []model.ShellHookService{model.NewZshHookService(), model.NewFishHookService(), model.NewBashHookService()} {
		if candidate.Match(env.shell) {
			svc = candidate
			break
		}
	}
	if svc == nil {
		return []doctorResult{{
			ID:      "hooks.shell",
			Status:  doctorWarn,
			Message: fmt.Sprintf("Your shell (%s) isn't supported, so its commands aren't tracked. shelltime supports zsh, bash and fish.", env.shell),
		}}
	}

	name := svc.ShellName()
	files := doctorShellFiles[name]
	fix := &doctorFix{
		Key:   "hooks." + name,
		Label: fmt.Sprintf("Install the %s hook (shelltime hooks install)", name),
		Run:   func(*cli.Context) error { return svc.Install() },
	}
	fixHint := "Run `shelltime hooks install`, then open a new terminal."
	results := []doctorResult{{ID: "hooks.shell", Status: doctorInfo, Message: fmt.Sprintf("Current shell: %s", env.shell)}}

	if err := svc.Check(); err != nil {
		results = append(results, doctorResult{
			ID:      "hooks.installed",
			Status:  doctorFail,
			Message: fmt.Sprintf("The %s hook isn't set up in %s, so the commands you run aren't tracked.", name, files.rc),
			Fix:     fixHint,
			AutoFix: fix,
		})
	} else {
		results = append(results, doctorResult{ID: "hooks.installed", Status: doctorOK, Message: fmt.Sprintf("The %s hook is set up in %s.", name, files.rc)})
	}

	hookPath := filepath.Join(model.GetHooksFolderPath(), files.hook)
	if _, err := os.Stat(hookPath); err != nil {
		results = append(results, doctorResult{
			ID:      "hooks.script",
			Status:  doctorFail,
			Message: fmt.Sprintf("The hook script %s is missing, so the hook in %s does nothing.", hookPath, files.rc),
			Fix:     fixHint,
			AutoFix: fix,
		})
	}

	if name == "bash" {
		preexecPath := filepath.Join(model.GetHooksFolderPath(), "bash-preexec.sh")
		if _, err := os.Stat(preexecPath); err != nil {
			results = append(results, doctorResult{
				ID:      "hooks.bash_preexec",
				Status:  doctorFail,
				Message: fmt.Sprintf("%s is missing; bash needs it to capture commands.", preexecPath),
				Fix:     fixHint,
				AutoFix: fix,
			})
		}
	}
	return results
}

// --- Claude Code --------------------------------------------------------------

func doctorClaudeDetected() bool {
	if _, err := doctorLookPath("claude"); err == nil {
		return true
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{".claude", ".claude/local/claude", ".local/bin/claude"} {
		if _, err := os.Stat(filepath.Join(home, p)); err == nil {
			return true
		}
	}
	return false
}

func doctorCheckClaude(env *doctorEnv) []doctorResult {
	if !doctorClaudeDetected() {
		return []doctorResult{{ID: "claude.detected", Status: doctorSkip, Message: "Claude Code wasn't found; skipped its checks."}}
	}

	svc := model.NewClaudeSettingsAICodeOtelEnvService()
	fix := &doctorFix{
		Key:   "claude.install",
		Label: "Set up Claude Code to report usage (shelltime cc install)",
		Run:   doctorRunCCInstall,
	}
	fixHint := "Run `shelltime cc install`, then restart Claude Code."

	var results []doctorResult
	if err := svc.Check(); err != nil && strings.Contains(err.Error(), "failed to parse") {
		results = append(results, doctorResult{
			ID:      "claude.otel",
			Status:  doctorFail,
			Message: fmt.Sprintf("%s isn't valid JSON, so Claude Code usage isn't reported: %v", svc.SettingsPath(), errors.Unwrap(err)),
			Fix:     fmt.Sprintf("Fix the JSON in %s, then run `shelltime cc install`.", svc.SettingsPath()),
		})
	} else if err != nil {
		results = append(results, doctorResult{
			ID:      "claude.otel",
			Status:  doctorFail,
			Message: "Claude Code isn't set up to report usage to shelltime, so your Claude Code sessions won't show up.",
			Fix:     fixHint,
			AutoFix: fix,
		})
	} else {
		env.claudeOtel = true
		results = append(results, doctorResult{ID: "claude.otel", Status: doctorOK, Message: fmt.Sprintf("Claude Code reports usage to shelltime (%s).", svc.SettingsPath())})

		// Settings written by an older `cc install` lack the newer keys (tool details,
		// assistant responses, delta temporality, ...).
		if missing, err := svc.MissingManagedKeys(); err == nil && len(missing) > 0 {
			results = append(results, doctorResult{
				ID:      "claude.otel_keys",
				Status:  doctorWarn,
				Message: fmt.Sprintf("Claude Code's shelltime OTEL config is out of date (missing %s), so some session details aren't reported.", strings.Join(missing, ", ")),
				Fix:     "Run `shelltime cc install` again, then restart Claude Code.",
				AutoFix: fix,
			})
		}
	}

	var legacyShells []string
	for _, legacy := range legacyAICodeOtelEnvServices() {
		if legacy.Check() == nil {
			legacyShells = append(legacyShells, legacy.ShellName())
		}
	}
	if len(legacyShells) > 0 {
		results = append(results, doctorResult{
			ID:      "claude.legacy_env",
			Status:  doctorWarn,
			Message: fmt.Sprintf("Your %s config still has the old shelltime OTEL block, which can override ~/.claude/settings.json.", strings.Join(legacyShells, ", ")),
			Fix:     "Run `shelltime cc install` to remove it.",
			AutoFix: fix,
		})
	}

	statusLineFix := fmt.Sprintf(`Add "statusLine": {"type": "command", "command": "%s"} to %s.`, doctorStatusLineCommand, svc.SettingsPath())
	switch cmd, err := svc.StatusLineCommand(); {
	case err != nil:
		results = append(results, doctorResult{ID: "claude.statusline", Status: doctorSkip, Message: fmt.Sprintf("Couldn't read the statusline setting: %v", err)})
	case strings.Contains(cmd, doctorStatusLineCommand):
		results = append(results, doctorResult{ID: "claude.statusline", Status: doctorOK, Message: "The shelltime statusline is on in Claude Code."})
	case cmd != "":
		results = append(results, doctorResult{
			ID:      "claude.statusline",
			Status:  doctorInfo,
			Message: fmt.Sprintf("Claude Code uses a different statusline (%s).", cmd),
			Fix:     fmt.Sprintf(`To show shelltime's cost and quota instead, set statusLine.command to "%s" in %s.`, doctorStatusLineCommand, svc.SettingsPath()),
		})
	default:
		results = append(results, doctorResult{
			ID:      "claude.statusline",
			Status:  doctorInfo,
			Message: "The shelltime statusline isn't on (optional: shows cost and quota inside Claude Code).",
			Fix:     statusLineFix,
		})
	}
	return results
}

// --- Codex --------------------------------------------------------------------

func doctorCodexDetected() bool {
	if _, err := doctorLookPath("codex"); err == nil {
		return true
	}
	home, _ := os.UserHomeDir()
	_, err := os.Stat(filepath.Join(home, ".codex"))
	return err == nil
}

func doctorCheckCodex(env *doctorEnv) []doctorResult {
	if !doctorCodexDetected() {
		return []doctorResult{{ID: "codex.detected", Status: doctorSkip, Message: "Codex wasn't found; skipped its checks."}}
	}

	fix := &doctorFix{
		Key:   "codex.install",
		Label: "Set up Codex to report usage (shelltime codex install)",
		Run:   doctorRunCodexInstall,
	}
	fixHint := "Run `shelltime codex install`, then restart Codex."

	result := doctorResult{ID: "codex.otel"}
	switch endpoint, err := model.NewCodexOtelConfigService().Endpoint(); {
	case err != nil:
		result.Status = doctorFail
		result.Message = fmt.Sprintf("~/.codex/config.toml can't be read, so Codex usage isn't reported: %v", err)
		result.Fix = "Fix the TOML syntax in ~/.codex/config.toml, then run `shelltime codex install`."
	case endpoint == "":
		result.Status = doctorFail
		result.Message = "Codex isn't set up to report usage to shelltime, so your Codex sessions won't show up."
		result.Fix = fixHint
		result.AutoFix = fix
	case endpoint != model.AICodeOtelEndpoint:
		result.Status = doctorFail
		result.Message = fmt.Sprintf("Codex sends usage to %s instead of shelltime's receiver (%s).", endpoint, model.AICodeOtelEndpoint)
		result.Fix = fixHint
		result.AutoFix = fix
	default:
		env.codexOtel = true
		result.Status = doctorOK
		result.Message = "Codex reports usage to shelltime."
	}
	results := []doctorResult{result}

	// A config written by an older `codex install` lacks log_agent_responses.
	if env.codexOtel {
		if missing, err := model.NewCodexOtelConfigService().MissingManagedKeys(); err == nil && len(missing) > 0 {
			results = append(results, doctorResult{
				ID:      "codex.otel_keys",
				Status:  doctorWarn,
				Message: fmt.Sprintf("Codex's shelltime OTEL config is out of date (missing otel.%s), so some session details aren't reported.", strings.Join(missing, ", otel.")),
				Fix:     "Run `shelltime codex install` again, then restart Codex.",
				AutoFix: fix,
			})
		}
	}

	if ok, _ := doctorCodexInstallationStatus(); !ok {
		results = append(results, doctorResult{
			ID:      "codex.auth",
			Status:  doctorInfo,
			Message: "Codex isn't signed in (no ~/.codex/auth.json), so your Codex usage limits can't be synced.",
			Fix:     "Run `codex login`.",
		})
	}
	return results
}

// --- AI usage receiver --------------------------------------------------------

func doctorCheckOtelReceiver(env *doctorEnv) []doctorResult {
	var tools []string
	if env.claudeOtel {
		tools = append(tools, "Claude Code")
	}
	if env.codexOtel {
		tools = append(tools, "Codex")
	}
	if len(tools) == 0 {
		return []doctorResult{{ID: "otel.enabled", Status: doctorSkip, Message: "No AI coding tool is set up to report usage; skipped."}}
	}
	if env.cfgErr != nil {
		return env.skipWithoutConfig("otel.enabled")
	}
	toolNames := strings.Join(tools, " and ")
	sends := "sends"
	if len(tools) > 1 {
		sends = "send"
	}
	configFile := env.configFileHint()

	otelCfg := env.cfg.AICodeOtel
	if otelCfg == nil || otelCfg.Enabled == nil || !*otelCfg.Enabled {
		return []doctorResult{{
			ID:      "otel.enabled",
			Status:  doctorFail,
			Message: fmt.Sprintf("%s %s usage to %s, but the daemon's receiver is turned off, so that usage is dropped.", toolNames, sends, model.AICodeOtelEndpoint),
			Fix:     fmt.Sprintf("Set `aiCodeOtel.enabled: true` in %s, then run `shelltime daemon reinstall`.", configFile),
		}}
	}
	results := []doctorResult{{ID: "otel.enabled", Status: doctorOK, Message: "The daemon's AI usage receiver is enabled."}}

	port := otelCfg.GRPCPort
	if port == 0 {
		port = model.DefaultAICodeOtelGRPCPort
	}
	if port != model.DefaultAICodeOtelGRPCPort {
		return append(results, doctorResult{
			ID:      "otel.port",
			Status:  doctorFail,
			Message: fmt.Sprintf("The receiver listens on port %d, but %s %s to port %d, so that usage is dropped.", port, toolNames, sends, model.DefaultAICodeOtelGRPCPort),
			Fix:     fmt.Sprintf("Remove `aiCodeOtel.grpcPort` from %s (or set it to %d), then run `shelltime daemon reinstall`.", configFile, model.DefaultAICodeOtelGRPCPort),
		})
	}

	if env.daemonStatus == nil {
		return append(results, doctorResult{ID: "otel.listening", Status: doctorSkip, Message: "Skipped: the daemon isn't running (see Daemon)."})
	}
	conn, err := doctorDialTCP("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return append(results, doctorResult{
			ID:      "otel.listening",
			Status:  doctorFail,
			Message: fmt.Sprintf("The daemon is running, but nothing is listening on port %d, so %s usage is dropped. The receiver may have been enabled after the daemon started, or another program holds the port.", port, toolNames),
			Fix:     fmt.Sprintf("Run `shelltime daemon reinstall`. If that doesn't help, check what uses the port with `lsof -i :%d`.", port),
			AutoFix: doctorDaemonInstallFix(env),
		})
	}
	conn.Close()
	return append(results, doctorResult{ID: "otel.listening", Status: doctorOK, Message: fmt.Sprintf("The receiver is listening on port %d.", port)})
}

// --- Sync ---------------------------------------------------------------------

func doctorCheckSync(env *doctorEnv) []doctorResult {
	var results []doctorResult

	pendingPath := model.GetSyncPendingFilePath()
	switch pending, err := countNonEmptyLines(pendingPath); {
	case err != nil && !os.IsNotExist(err):
		results = append(results, doctorResult{ID: "sync.pending", Status: doctorSkip, Message: fmt.Sprintf("Couldn't read %s: %v", pendingPath, err)})
	case pending > 0:
		results = append(results, doctorResult{
			ID:      "sync.pending",
			Status:  doctorWarn,
			Message: fmt.Sprintf("%d upload batch(es) failed and are queued in %s; the daemon retries them every hour.", pending, pendingPath),
			Fix:     "Fix any Account, Configuration or Daemon problems above; the queue uploads on its own once the server is reachable.",
		})
	default:
		results = append(results, doctorResult{ID: "sync.pending", Status: doctorOK, Message: "No failed uploads are waiting to be retried."})
	}

	if heartbeats, err := countNonEmptyLines(model.GetHeartbeatLogFilePath()); err == nil && heartbeats > 0 {
		results = append(results, doctorResult{
			ID:      "sync.heartbeats",
			Status:  doctorInfo,
			Message: fmt.Sprintf("%d editor heartbeat(s) are waiting to be re-sent; the daemon retries them every 30 minutes.", heartbeats),
		})
	}
	return results
}

// countNonEmptyLines counts lines with any non-whitespace content. It reads in chunks because
// queued sync batches can be far longer than bufio.Scanner's default line limit.
func countNonEmptyLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf := make([]byte, 64*1024)
	count, inLine := 0, false
	for {
		n, err := f.Read(buf)
		for _, b := range buf[:n] {
			switch b {
			case '\n':
				if inLine {
					count++
				}
				inLine = false
			case ' ', '\t', '\r':
			default:
				inLine = true
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
	}
	if inLine {
		count++
	}
	return count, nil
}
