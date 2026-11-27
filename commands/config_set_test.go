package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func switchKeys(switches []configSwitch) []string {
	keys := make([]string, len(switches))
	for i, sw := range switches {
		keys[i] = sw.key
	}
	return keys
}

func TestResolveConfigSwitches(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"ai.agent", "view,edit,delete"}, []string{"ai.agent.view", "ai.agent.edit", "ai.agent.delete"}},
		{[]string{"ai.agent", "view", "edit"}, []string{"ai.agent.view", "ai.agent.edit"}},
		{[]string{"ai.agent", "view, edit,view"}, []string{"ai.agent.view", "ai.agent.edit"}},
		{[]string{"ai.agent.view"}, []string{"ai.agent.view"}},
		{[]string{"AI.Agent", "Delete"}, []string{"ai.agent.delete"}},
		{[]string{"ai.showtips"}, []string{"ai.showTips"}},
		{[]string{"ai", "showTips,shareContext"}, []string{"ai.showTips", "ai.shareContext"}},
	}
	for _, c := range cases {
		got, err := resolveConfigSwitches(c.args)
		require.NoError(t, err, c.args)
		assert.Equal(t, c.want, switchKeys(got), c.args)
	}
}

func TestResolveConfigSwitchesErrors(t *testing.T) {
	_, err := resolveConfigSwitches(nil)
	assert.ErrorContains(t, err, "name a setting")

	_, err = resolveConfigSwitches([]string{"ai.agent"})
	assert.EqualError(t, err, "name the settings of ai.agent to change: view, edit, delete, e.g. `ai.agent view,edit`")

	_, err = resolveConfigSwitches([]string{"ai.agent", ","})
	assert.ErrorContains(t, err, "name the settings of ai.agent")

	_, err = resolveConfigSwitches([]string{"ai.agent", "view,run"})
	assert.ErrorContains(t, err, `unknown setting "ai.agent.run"`)

	_, err = resolveConfigSwitches([]string{"dataMasking"})
	assert.ErrorContains(t, err, `unknown setting "dataMasking"`)

	_, err = resolveConfigSwitches([]string{"ai.agent.view", "edit"})
	assert.EqualError(t, err, "ai.agent.view is a setting, not a section: name it alone, or the section first, e.g. `ai.agent view,edit`")
	_, err = resolveConfigSwitches([]string{"ai.showTips", "shareContext"})
	assert.ErrorContains(t, err, "e.g. `ai showTips,shareContext`")
}

func setupConfigSetTest(t *testing.T, config string) string {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER)
	require.NoError(t, os.MkdirAll(dir, 0755))
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0600))

	orig := configService
	configService = model.NewConfigService(dir)
	t.Cleanup(func() { configService = orig })
	return path
}

func TestConfigEnableAndDisable(t *testing.T) {
	path := setupConfigSetTest(t, "token: abc\n\n# ShellTime\nflushCount: 5\n")
	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigCommand}}

	require.NoError(t, app.Run([]string{"t", "config", "enable", "ai.agent", "view,edit,delete"}))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "token: abc\n\n# ShellTime\nflushCount: 5\nai:\n  agent:\n    view: true\n    edit: true\n    delete: true\n", string(data))

	require.NoError(t, app.Run([]string{"t", "config", "disable", "ai.agent", "delete"}))
	require.NoError(t, app.Run([]string{"t", "config", "disable", "ai.showTips"}))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "token: abc\n\n# ShellTime\nflushCount: 5\nai:\n  agent:\n    view: true\n    edit: true\n    delete: false\n  showTips: false\n", string(data))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "the token stays owner-only")
}

func TestConfigEnableErrors(t *testing.T) {
	path := setupConfigSetTest(t, "token: abc\n")
	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigCommand}}

	assert.Error(t, app.Run([]string{"t", "config", "enable", "ai.agent"}))
	assert.Error(t, app.Run([]string{"t", "config", "enable", "ai.agent", "run"}))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "token: abc\n", string(data), "nothing is written for a setting it doesn't know")

	require.NoError(t, os.Remove(path))
	err = app.Run([]string{"t", "config", "enable", "ai.agent.view"})
	assert.ErrorIs(t, err, model.ErrConfigNotFound)
}

func TestTakeQueryTipTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query-tip.count")
	orig := queryTipCountPath
	queryTipCountPath = func() string { return path }
	t.Cleanup(func() { queryTipCountPath = orig })

	var shown []int
	for call := 1; call <= 12; call++ {
		if takeQueryTipTurn() {
			shown = append(shown, call)
		}
	}
	assert.Equal(t, []int{5, 10}, shown)

	// A count that can't be read starts over
	require.NoError(t, os.WriteFile(path, []byte("garbage"), 0644))
	assert.False(t, takeQueryTipTurn())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "1\n", string(data))

	// Tips turned off don't count
	disabled := false
	require.NoError(t, os.WriteFile(path, []byte("4"), 0644))
	assert.False(t, showQueryTip(model.ShellTimeConfig{AI: &model.AIConfig{ShowTips: &disabled}}))
	assert.True(t, showQueryTip(model.ShellTimeConfig{}), "the call after four counted ones shows the tip")

	// A count that can't be saved shows nothing
	queryTipCountPath = func() string { return filepath.Join(path, "not-a-dir", "count") }
	for range queryTipEvery {
		assert.False(t, takeQueryTipTurn())
	}
}
