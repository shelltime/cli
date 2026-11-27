package model

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var agentView = []string{"ai", "agent", "view"}

func TestSetConfigBoolYAML(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		path  []string
		value bool
		want  string
	}{
		{
			name:  "adds the section after the last key, keeping blank lines and comments",
			in:    "token: abc\n\n# sync\nflushCount: 5\n",
			path:  agentView,
			value: true,
			want:  "token: abc\n\n# sync\nflushCount: 5\nai:\n  agent:\n    view: true\n",
		},
		{
			name:  "adds a key to an existing section before what follows it",
			in:    "token: abc\n\nai:\n  # tips\n  showTips: false\n\n# trailing\nexclude:\n  - foo\n",
			path:  []string{"ai", "agent", "edit"},
			value: true,
			want:  "token: abc\n\nai:\n  # tips\n  showTips: false\n  agent:\n    edit: true\n\n# trailing\nexclude:\n  - foo\n",
		},
		{
			name:  "replaces a value and keeps its comment",
			in:    "ai:\n  agent:\n    view: false # was off\n    edit: true\n",
			path:  agentView,
			value: true,
			want:  "ai:\n  agent:\n    view: true # was off\n    edit: true\n",
		},
		{
			name:  "follows the file's indent and fills the empty section shelltime init writes",
			in:    "token: abc\nai:\n    agent: {} # auto-run\n    showTips: null\n\naiCodeOtel:\n    enabled: true\n",
			path:  agentView,
			value: true,
			want:  "token: abc\nai:\n    agent: # auto-run\n        view: true\n    showTips: null\n\naiCodeOtel:\n    enabled: true\n",
		},
		{
			name:  "replaces null",
			in:    "ai:\n    showTips: null\n\nsocketPath: /tmp/shelltime.sock\n",
			path:  []string{"ai", "showTips"},
			value: false,
			want:  "ai:\n    showTips: false\n\nsocketPath: /tmp/shelltime.sock\n",
		},
		{
			name:  "fills a section left without a value",
			in:    "token: x\nai:\n\nfoo: 1\n",
			path:  agentView,
			value: true,
			want:  "token: x\nai:\n  agent:\n    view: true\n\nfoo: 1\n",
		},
		{
			name:  "fills a null section",
			in:    "token: x\nai: ~\nfoo: 1\n",
			path:  agentView,
			value: true,
			want:  "token: x\nai:\n  agent:\n    view: true\nfoo: 1\n",
		},
		{
			name:  "writes into a file with only comments",
			in:    "# hello",
			path:  []string{"ai", "showTips"},
			value: false,
			want:  "# hello\nai:\n  showTips: false\n",
		},
		{
			name:  "keeps CRLF line breaks",
			in:    "token: x\r\nai:\r\n  showTips: true\r\n",
			path:  agentView,
			value: true,
			want:  "token: x\r\nai:\r\n  showTips: true\r\n  agent:\r\n    view: true\r\n",
		},
		{
			name:  "encodes the document again for a flow mapping",
			in:    "ai: {agent: {view: false}}\n",
			path:  agentView,
			value: true,
			want:  "ai: {agent: {view: true}}\n",
		},
		{
			name:  "encodes the document again for a quoted value",
			in:    "# config\nai:\n  agent:\n    view: \"false\"\n",
			path:  agentView,
			value: true,
			want:  "# config\nai:\n  agent:\n    view: true\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := setConfigBool([]byte(c.in), formatYAML, c.path, c.value)
			require.NoError(t, err)
			assert.Equal(t, c.want, string(got))
		})
	}
}

func TestSetConfigBoolTOML(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		path  []string
		value bool
		want  string
	}{
		{
			name:  "adds a table at the end",
			in:    "Token = 'abc'\n\n[aiCodeOtel]\nenabled = true\n",
			path:  agentView,
			value: true,
			want:  "Token = 'abc'\n\n[aiCodeOtel]\nenabled = true\n\n[ai.agent]\nview = true\n",
		},
		{
			name:  "adds the key under its table",
			in:    "Token = 'abc'\n\n[ai.agent] # auto\nedit = true\n\n[aiCodeOtel]\nenabled = true\n",
			path:  agentView,
			value: true,
			want:  "Token = 'abc'\n\n[ai.agent] # auto\nview = true\nedit = true\n\n[aiCodeOtel]\nenabled = true\n",
		},
		{
			name:  "adds a dotted key under the parent table",
			in:    "[ai]\nshowTips = false\n",
			path:  agentView,
			value: true,
			want:  "[ai]\nagent.view = true\nshowTips = false\n",
		},
		{
			name:  "replaces a value and keeps its comment",
			in:    "[ai.agent]\nview = false # off\n",
			path:  agentView,
			value: true,
			want:  "[ai.agent]\nview = true # off\n",
		},
		{
			name:  "replaces a value in an inline table",
			in:    "[ai]\nagent = { view = false, edit = true }\n",
			path:  agentView,
			value: true,
			want:  "[ai]\nagent = { view = true, edit = true }\n",
		},
		{
			name:  "adds a dotted key at the top next to other dotted keys",
			in:    "ai.agent.edit = true\n[aiCodeOtel]\nenabled = true\n",
			path:  agentView,
			value: true,
			want:  "ai.agent.view = true\nai.agent.edit = true\n[aiCodeOtel]\nenabled = true\n",
		},
		{
			name:  "writes into an empty file",
			in:    "",
			path:  agentView,
			value: true,
			want:  "[ai.agent]\nview = true\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := setConfigBool([]byte(c.in), formatTOML, c.path, c.value)
			require.NoError(t, err)
			assert.Equal(t, c.want, string(got))
		})
	}
}

func TestSetConfigBoolRefusesWhatItCantWriteCleanly(t *testing.T) {
	// An inline table can't take another key without rewriting it
	_, err := setConfigBool([]byte("[ai]\nagent = { edit = true }\n"), formatTOML, agentView, true)
	assert.ErrorContains(t, err, "set it to true there by hand")

	// ai is not a section
	_, err = setConfigBool([]byte("ai: on\n"), formatYAML, agentView, true)
	assert.ErrorContains(t, err, "set it to true there by hand")

	// Encoding again would drop the documents after the first
	_, err = setConfigBool([]byte("ai: {agent: {view: false}}\n---\nfoo: 1\n"), formatYAML, agentView, true)
	assert.ErrorContains(t, err, "set it to true there by hand")
	// A patch still works there
	got, err := setConfigBool([]byte("ai:\n  agent:\n    view: false\n---\nfoo: 1\n"), formatYAML, agentView, true)
	require.NoError(t, err)
	assert.Equal(t, "ai:\n  agent:\n    view: true\n---\nfoo: 1\n", string(got))

	_, err = setConfigBool([]byte("ai: [\n"), formatYAML, agentView, true)
	assert.ErrorContains(t, err, "failed to parse config file")
	_, err = setConfigBool([]byte("[ai\n"), formatTOML, agentView, true)
	assert.ErrorContains(t, err, "failed to parse config file")
}

func TestWriteConfigBoolsWritesTheFileThatDecides(t *testing.T) {
	t.Run("the base file without a local one", func(t *testing.T) {
		dir := t.TempDir()
		base := writeConfigFixture(t, dir, "config.yaml", "token: abc\n", 0600)

		files, err := WriteConfigBools(dir, []ConfigBool{
			{Path: agentView, Value: true},
			{Path: []string{"ai", "agent", "edit"}, Value: true},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{base}, files)
		assert.Equal(t, "token: abc\nai:\n  agent:\n    view: true\n    edit: true\n", readFixture(t, base))
		assertConfigBool(t, dir, true)
	})

	t.Run("the local file when it has the section", func(t *testing.T) {
		dir := t.TempDir()
		base := writeConfigFixture(t, dir, "config.yaml", "token: abc\nai:\n  showTips: false\n", 0644)
		local := writeConfigFixture(t, dir, "config.local.yaml", "ai:\n  shareContext: false\n", 0644)

		files, err := WriteConfigBools(dir, []ConfigBool{{Path: agentView, Value: true}})
		require.NoError(t, err)
		assert.Equal(t, []string{local}, files)
		assert.Equal(t, "token: abc\nai:\n  showTips: false\n", readFixture(t, base))
		assert.Equal(t, "ai:\n  shareContext: false\n  agent:\n    view: true\n", readFixture(t, local))
		assertConfigBool(t, dir, true)
	})

	t.Run("the base file when the local file doesn't load, as ReadConfigFile skips it", func(t *testing.T) {
		dir := t.TempDir()
		base := writeConfigFixture(t, dir, "config.yaml", "token: abc\n", 0644)
		local := writeConfigFixture(t, dir, "config.local.yaml", "flushCount: nope\nai:\n  showTips: false\n", 0644)

		files, err := WriteConfigBools(dir, []ConfigBool{{Path: agentView, Value: true}})
		require.NoError(t, err)
		assert.Equal(t, []string{base}, files)
		assert.Equal(t, "flushCount: nope\nai:\n  showTips: false\n", readFixture(t, local))
		assertConfigBool(t, dir, true)
	})

	t.Run("the local file when its TOML section is spelled in another case", func(t *testing.T) {
		dir := t.TempDir()
		writeConfigFixture(t, dir, "config.yaml", "token: abc\n", 0644)
		local := writeConfigFixture(t, dir, "config.local.toml", "[AI]\nshowTips = false\n", 0644)

		files, err := WriteConfigBools(dir, []ConfigBool{{Path: agentView, Value: true}})
		require.NoError(t, err)
		assert.Equal(t, []string{local}, files)
		assertConfigBool(t, dir, true)
	})

	t.Run("the base file when the local file leaves the section empty", func(t *testing.T) {
		dir := t.TempDir()
		base := writeConfigFixture(t, dir, "config.toml", "Token = 'abc'\n", 0644)
		writeConfigFixture(t, dir, "config.local.yaml", "token: local\nai:\n", 0644)

		files, err := WriteConfigBools(dir, []ConfigBool{{Path: agentView, Value: true}})
		require.NoError(t, err)
		assert.Equal(t, []string{base}, files)
		assertConfigBool(t, dir, true)
	})
}

func TestWriteConfigBoolsKeepsTheFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes and symlinks")
	}
	dir := t.TempDir()
	repo := t.TempDir()
	target := writeConfigFixture(t, repo, "shelltime.yaml", "token: abc\n", 0640)
	link := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.Symlink(target, link))

	_, err := WriteConfigBools(dir, []ConfigBool{{Path: agentView, Value: true}})
	require.NoError(t, err)

	linkInfo, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, linkInfo.Mode()&os.ModeSymlink, "a config linked from a dotfiles repo stays a link")
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0640), info.Mode().Perm())
	assert.Equal(t, "token: abc\nai:\n  agent:\n    view: true\n", readFixture(t, target))

	entries, err := os.ReadDir(repo)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

func TestWriteConfigBoolsInAFolderThatTakesNoNewFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs folder permissions that apply")
	}
	dir := t.TempDir()
	base := writeConfigFixture(t, dir, "config.yaml", "token: abc\n", 0644)
	require.NoError(t, os.Chmod(dir, 0555))
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	_, err := WriteConfigBools(dir, []ConfigBool{{Path: agentView, Value: true}})
	require.NoError(t, err)
	assert.Equal(t, "token: abc\nai:\n  agent:\n    view: true\n", readFixture(t, base))
}

func TestWriteConfigBoolsErrors(t *testing.T) {
	_, err := WriteConfigBools(t.TempDir(), []ConfigBool{{Path: agentView, Value: true}})
	assert.ErrorIs(t, err, ErrConfigNotFound)

	dir := t.TempDir()
	base := writeConfigFixture(t, dir, "config.toml", "[ai]\nagent = { edit = true }\n", 0644)
	_, err = WriteConfigBools(dir, []ConfigBool{{Path: agentView, Value: true}})
	assert.ErrorContains(t, err, base)
	assert.Equal(t, "[ai]\nagent = { edit = true }\n", readFixture(t, base), "a refused change writes nothing")
}

func writeConfigFixture(t *testing.T, dir, name, content string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), perm))
	require.NoError(t, os.Chmod(path, perm))
	return path
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func assertConfigBool(t *testing.T, dir string, view bool) {
	t.Helper()
	cfg, err := NewConfigService(dir).ReadConfigFile(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cfg.AI)
	assert.Equal(t, view, cfg.AI.Agent.View)
}
