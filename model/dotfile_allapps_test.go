package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllAppsMap_AllPresentAndPopulated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	apps := GetAllAppsMap()

	// Every declared app name must be present in the map.
	require.Len(t, apps, len(AllAvailableApps))
	for _, name := range AllAvailableApps {
		app, ok := apps[name]
		require.True(t, ok, "missing app %q", name)
		require.NotNil(t, app, "nil app for %q", name)
		assert.Equal(t, string(name), app.Name(), "Name() should match map key")
		assert.NotEmpty(t, app.GetConfigPaths(), "%q should declare config paths", name)
	}
}

func TestAllApps_ConstructorsAndNames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := []struct {
		app  DotfileApp
		name string
	}{
		{NewNvimApp(), "nvim"},
		{NewFishApp(), "fish"},
		{NewGitApp(), "git"},
		{NewZshApp(), "zsh"},
		{NewBashApp(), "bash"},
		{NewGhosttyApp(), "ghostty"},
		{NewClaudeApp(), "claude"},
		{NewStarshipApp(), "starship"},
		{NewNpmApp(), "npm"},
		{NewSshApp(), "ssh"},
		{NewKittyApp(), "kitty"},
		{NewKubernetesApp(), "kubernetes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.app)
			assert.Equal(t, tc.name, tc.app.Name())
			assert.NotEmpty(t, tc.app.GetConfigPaths())
			// GetIncludeDirectives may be nil (apps without include support) but
			// must not panic.
			assert.NotPanics(t, func() { _ = tc.app.GetIncludeDirectives() })
		})
	}
}

func TestApps_CollectDotfiles_FromTempHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Claude uses plain CollectFromPaths (no include support); create its files.
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"a":1}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("# guide"), 0o644))

	claude := NewClaudeApp()
	items, err := claude.CollectDotfiles(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	paths := map[string]bool{}
	for _, it := range items {
		assert.Equal(t, "claude", it.App)
		assert.Equal(t, "file", it.FileType)
		paths[filepath.Base(it.Path)] = true
	}
	assert.True(t, paths["settings.json"])
	assert.True(t, paths["CLAUDE.md"])
}

func TestApps_CollectDotfiles_BashWithIncludeSupport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Bash collects via CollectWithIncludeSupport: it copies the original
	// ~/.bashrc content into ~/.bashrc.shelltime, adds an include line to the
	// original, and collects from the .shelltime companion file.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("export A=1\n"), 0o644))

	bash := NewBashApp()
	items, err := bash.CollectDotfiles(context.Background())
	require.NoError(t, err)

	found := false
	for _, it := range items {
		assert.Equal(t, "bash", it.App)
		if filepath.Base(it.Path) == ".bashrc.shelltime" {
			found = true
			assert.Contains(t, it.Content, "export A=1")
		}
	}
	assert.True(t, found, "expected .bashrc.shelltime companion to be collected")

	// The original .bashrc should now carry the include line.
	orig, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	require.NoError(t, err)
	assert.Contains(t, string(orig), ".bashrc.shelltime")
}

// TestApps_CollectAndSave_PrimaryConfig creates the primary config file for
// each app, runs CollectDotfiles (covers the per-app collect wrapper) and Save
// (covers the per-app save wrapper) and asserts the content round-trips. Apps
// with include support write to a .shelltime companion; apps without it write
// the original path directly. We assert behavior generically.
func TestApps_CollectAndSave_PrimaryConfig(t *testing.T) {
	type appCase struct {
		name        string
		make        func() DotfileApp
		primaryRel  string // primary config file path relative to HOME
		hasInclude  bool   // include-support apps collect from .shelltime
		shelltime   string // expected .shelltime companion (relative to HOME) when hasInclude
		saveContent string
	}
	cases := []appCase{
		{"git", NewGitApp, ".gitconfig", true, ".gitconfig.shelltime", "[user]\n  name = me\n"},
		{"npm", NewNpmApp, ".npmrc", false, "", "registry=https://example.com\n"},
		{"starship", NewStarshipApp, ".config/starship.toml", false, "", "add_newline = false\n"},
		{"kubernetes", NewKubernetesApp, ".kube/config", false, "", "apiVersion: v1\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			primary := filepath.Join(home, tc.primaryRel)
			require.NoError(t, os.MkdirAll(filepath.Dir(primary), 0o755))
			require.NoError(t, os.WriteFile(primary, []byte("original\n"), 0o644))

			app := tc.make()

			items, err := app.CollectDotfiles(context.Background())
			require.NoError(t, err)
			require.NotEmpty(t, items, "expected to collect at least one item")
			for _, it := range items {
				assert.Equal(t, tc.name, it.App)
			}

			// Save new content; for non-include apps it diff-merges/creates the
			// primary path, for include apps the .shelltime companion is written.
			savePath := "~/" + tc.primaryRel
			if tc.hasInclude {
				savePath = "~/" + tc.shelltime
			}
			require.NoError(t, app.Save(context.Background(), map[string]string{savePath: tc.saveContent}, false))

			// Verify the saved content was written for the save target. Include
			// apps overwrite the .shelltime companion directly; non-include apps
			// diff-merge the new content into the original file.
			target := filepath.Join(home, tc.primaryRel)
			if tc.hasInclude {
				target = filepath.Join(home, tc.shelltime)
			}
			written, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Contains(t, string(written), tc.saveContent, "Save should persist the supplied content")
		})
	}
}

// TestApps_Collect_DirectoryConfigs covers apps whose config path is a directory
// (fish, nvim, zsh, kitty) by populating files inside the directory.
func TestApps_Collect_DirectoryConfigs(t *testing.T) {
	cases := []struct {
		name    string
		make    func() DotfileApp
		dirRel  string
		fileRel string
	}{
		{"kitty", NewKittyApp, ".config/kitty", "kitty.conf"},
		{"fish", NewFishApp, ".config/fish/functions", "fn.fish"},
		{"nvim", NewNvimApp, ".config/nvim", "init.lua"},
		{"zsh", NewZshApp, ".config/zsh", "extra.zsh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			dir := filepath.Join(home, tc.dirRel)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, tc.fileRel), []byte("content\n"), 0o644))

			app := tc.make()
			items, err := app.CollectDotfiles(context.Background())
			require.NoError(t, err)
			found := false
			for _, it := range items {
				assert.Equal(t, tc.name, it.App)
				if filepath.Base(it.Path) == tc.fileRel {
					found = true
					assert.Equal(t, "content\n", it.Content)
				}
			}
			assert.True(t, found, "expected %s to be collected from %s", tc.fileRel, tc.dirRel)
		})
	}
}

// TestIncludeApps_CollectAndSave covers CollectDotfiles + Save for the
// include-directive apps that wrap a single original file (ssh) and the shell
// apps (bash/fish/nvim/zsh) whose Save writes the .shelltime companion.
func TestIncludeApps_CollectAndSave(t *testing.T) {
	cases := []struct {
		name      string
		make      func() DotfileApp
		origRel   string // original config (relative to HOME)
		stRel     string // .shelltime companion (relative to HOME)
		checkSub  string // include substring expected in original after setup
	}{
		{"ssh", NewSshApp, ".ssh/config", ".ssh/config.shelltime", "config.shelltime"},
		{"fish", NewFishApp, ".config/fish/config.fish", ".config/fish/config.fish.shelltime", "config.fish.shelltime"},
		{"nvim", NewNvimApp, ".vimrc", ".vimrc.shelltime", ".vimrc.shelltime"},
		{"zsh", NewZshApp, ".zshrc", ".zshrc.shelltime", ".zshrc.shelltime"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			orig := filepath.Join(home, tc.origRel)
			require.NoError(t, os.MkdirAll(filepath.Dir(orig), 0o755))
			require.NoError(t, os.WriteFile(orig, []byte("original-config\n"), 0o644))

			app := tc.make()

			// Collect should set up the include and read the .shelltime companion.
			items, err := app.CollectDotfiles(context.Background())
			require.NoError(t, err)
			gotCompanion := false
			for _, it := range items {
				assert.Equal(t, tc.name, it.App)
				if filepath.Base(it.Path) == filepath.Base(tc.stRel) {
					gotCompanion = true
					assert.Contains(t, it.Content, "original-config")
				}
			}
			assert.True(t, gotCompanion, "expected .shelltime companion collected")

			// Original now has the include line.
			origAfter, err := os.ReadFile(orig)
			require.NoError(t, err)
			assert.Contains(t, string(origAfter), tc.checkSub)

			// Save writes server content into the .shelltime companion.
			require.NoError(t, app.Save(context.Background(),
				map[string]string{"~/" + tc.stRel: "managed-by-server\n"}, false))
			st, err := os.ReadFile(filepath.Join(home, tc.stRel))
			require.NoError(t, err)
			assert.Contains(t, string(st), "managed-by-server")
		})
	}
}

// TestBashApp_Save covers BashApp.Save via the .shelltime companion path.
func TestBashApp_Save(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("export A=1\n"), 0o644))

	bash := NewBashApp()
	require.NoError(t, bash.Save(context.Background(),
		map[string]string{"~/.bashrc.shelltime": "export FROM_SERVER=1\n"}, false))

	st, err := os.ReadFile(filepath.Join(home, ".bashrc.shelltime"))
	require.NoError(t, err)
	assert.Contains(t, string(st), "export FROM_SERVER=1")
	// Original bashrc should carry the include line.
	orig, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	require.NoError(t, err)
	assert.Contains(t, string(orig), ".bashrc.shelltime")
}

func TestGhostty_CollectDotfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := filepath.Join(home, ".config", "ghostty", "config")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfg), 0o755))
	require.NoError(t, os.WriteFile(cfg, []byte("font-size = 14\n"), 0o644))

	g := NewGhosttyApp()
	items, err := g.CollectDotfiles(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "ghostty", items[0].App)
	assert.Contains(t, items[0].Content, "font-size = 14")
}

func TestApps_IsEqual_DelegatesToBaseApp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("export A=1\n"), 0o644))

	bash := NewBashApp()
	result, err := bash.IsEqual(context.Background(), map[string]string{
		"~/.bashrc": "export A=1\n",
	})
	require.NoError(t, err)
	assert.True(t, result["~/.bashrc"])
}
