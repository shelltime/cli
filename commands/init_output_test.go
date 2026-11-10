package commands

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI drops color codes and the surrounding whitespace.
func stripANSI(s string) string {
	return strings.TrimSpace(ansiEscape.ReplaceAllString(s, ""))
}

// captureColor sends everything printed through gookit/color to the returned buffer.
func captureColor(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	color.SetOutput(&buf)
	t.Cleanup(color.ResetOutput)
	return &buf
}

// --- hooks install --------------------------------------------------------------

// A machine with only bash gets the bash hook and no word about zsh or fish failing.
func TestCommandHooksInstall_OnlyBash(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	base := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER)
	require.NoError(t, os.MkdirAll(filepath.Join(base, "bin"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "hooks"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "hooks", "bash-preexec.sh"), []byte("# stub\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# mine\n"), 0644))

	out := captureColor(t)
	app := &cli.App{Name: "t", Commands: []*cli.Command{HooksInstallCommand}}
	require.NoError(t, app.Run([]string{"t", "install"}))

	text := out.String()
	assert.Contains(t, text, "Installed the bash hook in ~/.bashrc")
	assert.Contains(t, text, "Skipped zsh and fish: no config file, and not your login shell.")
	assert.NotContains(t, text, "Failed")
	assert.NotContains(t, text, "❌")
	assert.NoFileExists(t, filepath.Join(home, ".zshrc"))
	assert.NoDirExists(t, filepath.Join(home, ".config", "fish"))
}

func TestPrintHookResults(t *testing.T) {
	t.Setenv("HOME", "/home/u")

	t.Run("one line per shell", func(t *testing.T) {
		out := captureColor(t)
		printHookResults([]model.HookInstallResult{
			{Shell: "zsh", ConfigPath: "/home/u/.zshrc", Status: model.HookAlreadyInstalled},
			{Shell: "fish", ConfigPath: "/home/u/.config/fish/config.fish", Status: model.HookFailed, Err: errors.New("disk full")},
			{Shell: "bash", ConfigPath: "/home/u/.bashrc", Status: model.HookSkipped},
		})
		text := out.String()
		assert.Contains(t, text, "The zsh hook is already installed in ~/.zshrc")
		assert.Contains(t, text, "Failed to install the fish hook: disk full")
		assert.Contains(t, text, "Skipped bash: no config file, and not your login shell.")
	})

	t.Run("no shell found", func(t *testing.T) {
		out := captureColor(t)
		printHookResults([]model.HookInstallResult{
			{Shell: "zsh", ConfigPath: "/home/u/.zshrc", Status: model.HookSkipped},
			{Shell: "bash", ConfigPath: "/home/u/.bashrc", Status: model.HookSkipped},
		})
		text := out.String()
		assert.Contains(t, text, "No shell hook installed: none of ~/.zshrc, ~/.bashrc exists")
		assert.Contains(t, text, "shelltime hooks install")
		assert.NotContains(t, text, "Skipped")
	})
}

func TestPrintHookSummary(t *testing.T) {
	t.Setenv("HOME", "/home/u")

	t.Run("names the hooked shells only", func(t *testing.T) {
		out := captureColor(t)
		printHookSummary([]model.HookInstallResult{
			{Shell: "zsh", ConfigPath: "/home/u/.zshrc", Status: model.HookInstalled},
			{Shell: "fish", ConfigPath: "/home/u/.config/fish/config.fish", Status: model.HookSkipped},
			{Shell: "bash", ConfigPath: "/home/u/.bashrc", Status: model.HookAlreadyInstalled},
		})
		assert.Equal(t, "✅ Shell hooks installed for zsh and bash", stripANSI(out.String()))
	})

	t.Run("reports failures", func(t *testing.T) {
		out := captureColor(t)
		printHookSummary([]model.HookInstallResult{
			{Shell: "zsh", ConfigPath: "/home/u/.zshrc", Status: model.HookFailed, Err: errors.New("denied")},
			{Shell: "bash", ConfigPath: "/home/u/.bashrc", Status: model.HookSkipped},
		})
		text := out.String()
		assert.Contains(t, text, "Failed to install the zsh hook: denied")
		assert.NotContains(t, text, "No shell hook installed", "a failure already says what went wrong")
	})

	t.Run("no shell found", func(t *testing.T) {
		out := captureColor(t)
		printHookSummary([]model.HookInstallResult{
			{Shell: "bash", ConfigPath: "/home/u/.bashrc", Status: model.HookSkipped},
		})
		assert.Contains(t, out.String(), "No shell hook installed")
	})
}

func TestJoinWithAnd(t *testing.T) {
	assert.Equal(t, "", joinWithAnd(nil))
	assert.Equal(t, "bash", joinWithAnd([]string{"bash"}))
	assert.Equal(t, "zsh and bash", joinWithAnd([]string{"zsh", "bash"}))
	assert.Equal(t, "zsh, fish and bash", joinWithAnd([]string{"zsh", "fish", "bash"}))
}

func TestTildePath(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	assert.Equal(t, "~/.bashrc", tildePath("/home/u/.bashrc"))
	assert.Equal(t, "/home/user2/.bashrc", tildePath("/home/user2/.bashrc"))
	assert.Equal(t, "/etc/bashrc", tildePath("/etc/bashrc"))
}

// --- init ------------------------------------------------------------------------

func TestRunInitStep(t *testing.T) {
	step := func(err error) func() error {
		return func() error {
			color.Yellow.Println("🔄 Reloading systemd...")
			return err
		}
	}
	replayed := func(t *testing.T) *bytes.Buffer {
		var buf bytes.Buffer
		orig := initStepOutput
		initStepOutput = &buf
		t.Cleanup(func() { initStepOutput = orig })
		return &buf
	}

	t.Run("quiet success prints nothing", func(t *testing.T) {
		out := replayed(t)
		require.NoError(t, runInitStep(false, step(nil)))
		assert.Empty(t, out.String())
	})

	t.Run("quiet failure prints what the step printed", func(t *testing.T) {
		out := replayed(t)
		err := runInitStep(false, step(assert.AnError))
		assert.ErrorIs(t, err, assert.AnError)
		assert.Contains(t, out.String(), "Reloading systemd...")
	})

	t.Run("verbose prints as the step goes", func(t *testing.T) {
		out := replayed(t)
		live := captureColor(t)
		require.NoError(t, runInitStep(true, step(nil)))
		assert.Contains(t, live.String(), "Reloading systemd...")
		assert.Empty(t, out.String())
	})
}

func TestPrintAgentTelemetrySummary(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name            string
		verbose         bool
		ccErr, codexErr error
		want, notWant   []string
	}{
		{name: "both", want: []string{"Claude Code and Codex telemetry configured"}},
		{name: "codex failed", codexErr: boom, want: []string{"✅ Claude Code telemetry configured", "Failed to install Codex OTEL config: boom"}},
		{name: "claude code failed", ccErr: boom, want: []string{"✅ Codex telemetry configured", "Failed to install Claude Code OTEL config: boom"}},
		{name: "verbose has no summary", verbose: true, notWant: []string{"telemetry configured"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureColor(t)
			printAgentTelemetrySummary(tt.verbose, tt.ccErr, tt.codexErr)
			for _, w := range tt.want {
				assert.Contains(t, out.String(), w)
			}
			for _, w := range tt.notWant {
				assert.NotContains(t, out.String(), w)
			}
			assert.NotContains(t, out.String(), "Privacy")
		})
	}
}

// --- gc ----------------------------------------------------------------------------

func TestCommandGC_SetupHint(t *testing.T) {
	tests := []struct {
		name     string
		cfgErr   error
		terminal bool
		wantHint bool
	}{
		{name: "no config, terminal", cfgErr: fmt.Errorf("%w in /x", model.ErrConfigNotFound), terminal: true, wantHint: true},
		{name: "no config, script", cfgErr: fmt.Errorf("%w in /x", model.ErrConfigNotFound), terminal: false},
		{name: "broken config", cfgErr: errors.New("failed to parse config file"), terminal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir, _, mc := setupGCTest(t)
			require.NoError(t, os.MkdirAll(baseDir, 0755))
			mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, tt.cfgErr)

			var stderr bytes.Buffer
			origOut, origTerm := gcStderr, gcStderrIsTerminal
			gcStderr, gcStderrIsTerminal = &stderr, func() bool { return tt.terminal }
			t.Cleanup(func() { gcStderr, gcStderrIsTerminal = origOut, origTerm })

			app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
			require.NoError(t, app.Run([]string{"t", "gc", "--skipLogCreation"}))

			if tt.wantHint {
				assert.Equal(t, setupHint, stripANSI(stderr.String()))
			} else {
				assert.Empty(t, stderr.String())
			}
		})
	}
}
