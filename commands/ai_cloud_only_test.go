package commands

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// aiCloudOnlyCases turns the mode on through the config, then through the Claude Code cloud
// environment with the key unset.
var aiCloudOnlyCases = []struct {
	name   string
	cfg    model.ShellTimeConfig
	remote string
}{
	{name: "config", cfg: model.ShellTimeConfig{AICloudOnly: new(true)}},
	{name: "cloud environment", remote: "true"},
}

// uploadRecorder is an API server that counts the requests it gets.
func uploadRecorder(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

// Without a daemon, `track` stores nothing in AI cloud only mode, so a later sync has nothing
// to upload.
func TestCommandTrack_AICloudOnlyStoresNothing(t *testing.T) {
	for _, tt := range aiCloudOnlyCases {
		t.Run(tt.name, func(t *testing.T) {
			_, mc := x3SetupTrack(t)
			if tt.remote != "" {
				t.Setenv("CLAUDE_CODE_REMOTE", tt.remote)
			}
			srv, requests := uploadRecorder(t)
			cfg := tt.cfg
			cfg.Token, cfg.APIEndpoint, cfg.FlushCount = "tok", srv.URL, 3
			cfg.SocketPath = x3UnreadySocket(t)
			mc.On("ReadConfigFile", mock.Anything).Return(cfg, nil)

			app := &cli.App{Name: "t", Commands: []*cli.Command{TrackCommand}}
			for _, phase := range []string{"pre", "post"} {
				require.NoError(t, app.Run([]string{"t", "track", "--phase", phase, "--shell", "bash", "--command", "go test ./...", "--id", "1"}))
			}

			_, err := os.Stat(model.GetCommandsStoragePath())
			assert.True(t, os.IsNotExist(err), "no command buffer is written")
			assert.Zero(t, requests.Load())
		})
	}
}

// sync, alias import and dotfiles push send commands or describe this machine, which would make
// it a host: in AI cloud only mode they say so and send nothing.
func TestUploadCommands_AICloudOnlySendNothing(t *testing.T) {
	runs := map[string]struct {
		cmd  *cli.Command
		args []string
	}{
		"sync":          {cmd: SyncCommand, args: []string{"t", "sync"}},
		"alias import":  {cmd: AliasCommand, args: []string{"t", "alias", "import"}},
		"dotfiles push": {cmd: DotfilesCommand, args: []string{"t", "dotfiles", "push"}},
	}
	for name, run := range runs {
		for _, tt := range aiCloudOnlyCases {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				mc := setupAliasTest(t)
				home := t.TempDir()
				t.Setenv("HOME", home)
				if tt.remote != "" {
					t.Setenv("CLAUDE_CODE_REMOTE", tt.remote)
				}
				// Something to upload for each command, if it ran
				require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("alias gs='git status'\n"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("export A=1\n"), 0644))
				buffered := model.Command{Shell: "bash", SessionID: 1, Command: "go test ./...", Hostname: "vm", Username: "root", Time: time.Now()}
				require.NoError(t, buffered.DoSavePre())
				buffered.Time = buffered.Time.Add(time.Second)
				require.NoError(t, buffered.DoUpdate(0))

				srv, requests := uploadRecorder(t)
				cfg := tt.cfg
				cfg.Token, cfg.APIEndpoint = "tok", srv.URL
				cfg.SocketPath = x3UnreadySocket(t)
				mc.On("ReadConfigFile", mock.Anything).Return(cfg, nil)

				app := &cli.App{Name: "t", Commands: []*cli.Command{run.cmd}}
				require.NoError(t, app.Run(run.args))
				assert.Zero(t, requests.Load())
			})
		}
	}
}

// hooks install can run before signing in, so it reads the config itself: off in the config wins
// over the cloud environment's default.
func TestCommandHooksInstall_AICloudOnly(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		remote    string
		installed bool
	}{
		{name: "cloud environment without a config", remote: "true", installed: false},
		{name: "on in the config", config: "aiCloudOnly: true\n", installed: false},
		{name: "off in the config in the cloud", config: "aiCloudOnly: false\n", remote: "true", installed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SKIP_LOGGER_SETTINGS = true
			home := t.TempDir()
			t.Setenv("HOME", home)
			if tt.remote != "" {
				t.Setenv("CLAUDE_CODE_REMOTE", tt.remote)
			}
			base := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER)
			require.NoError(t, os.MkdirAll(filepath.Join(base, "bin"), 0755))
			// bash-preexec.sh is pre-seeded so installing the bash hook needs no network
			require.NoError(t, os.MkdirAll(filepath.Join(base, "hooks"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(base, "hooks", "bash-preexec.sh"), []byte("# stub\n"), 0644))
			if tt.config != "" {
				require.NoError(t, os.WriteFile(filepath.Join(base, "config.yaml"), []byte("token: tok\n"+tt.config), 0600))
			}

			app := &cli.App{Name: "t", Commands: []*cli.Command{HooksInstallCommand}}
			require.NoError(t, app.Run([]string{"t", "install"}))

			_, err := os.Stat(filepath.Join(home, ".bashrc"))
			assert.Equal(t, tt.installed, err == nil, "bash hook installed")
		})
	}
}

// doctor names the mode and where it comes from, and doesn't ask for shell hooks.
func TestDoctor_AICloudOnly(t *testing.T) {
	home, mc := setupDoctorTest(t)
	writeDoctorConfigFile(t, home)
	t.Setenv("CLAUDE_CODE_REMOTE", "true")
	cfg := doctorTestConfig(t)
	mc.On("ReadConfigFile", mock.Anything).Return(cfg, nil)

	env := newDoctorEnv(context.Background(), doctorOptions{offline: true})
	require.True(t, env.aiCloudOnly)
	mode := findDoctorResult(t, doctorCheckConfig(env), "config.ai_cloud_only")
	assert.Equal(t, doctorInfo, mode.Status)
	assert.Contains(t, mode.Message, "Claude Code cloud environment")
	hooks := doctorCheckHooks(env)
	require.Len(t, hooks, 1)
	assert.Equal(t, doctorSkip, hooks[0].Status)

	env.cfg.AICloudOnly = new(true)
	assert.Contains(t, findDoctorResult(t, doctorCheckConfig(env), "config.ai_cloud_only").Message, "in your config")

	env.aiCloudOnly = false
	assert.False(t, hasDoctorResult(doctorCheckConfig(env), "config.ai_cloud_only"))
	assert.NotEqual(t, doctorSkip, findDoctorResult(t, doctorCheckHooks(env), "hooks.shell").Status)
}
