package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGhostty_BasicMetadata(t *testing.T) {
	app := NewGhosttyApp()
	assert.Equal(t, "ghostty", app.Name())
	assert.Equal(t, []string{"~/.config/ghostty/config"}, app.GetConfigPaths())
	assert.Nil(t, app.GetIncludeDirectives())
}

func TestGhostty_parseGhosttyConfig(t *testing.T) {
	g := &GhosttyApp{}
	content := "# a comment\n\nfont-size = 14\ntheme=dark\nstandalone-word\n"
	lines := g.parseGhosttyConfig(content)

	require.Len(t, lines, 5)

	assert.True(t, lines[0].isComment)
	assert.Equal(t, "# a comment", lines[0].raw)

	assert.True(t, lines[1].isBlank)

	assert.False(t, lines[2].isComment)
	assert.Equal(t, "font-size", lines[2].key)
	assert.Equal(t, "14", lines[2].value)

	// no spaces around '='
	assert.Equal(t, "theme", lines[3].key)
	assert.Equal(t, "dark", lines[3].value)

	// a line without '=' is treated as a comment
	assert.True(t, lines[4].isComment)
	assert.Equal(t, "standalone-word", lines[4].raw)
}

func TestGhostty_mergeGhosttyConfigs_localWins(t *testing.T) {
	g := &GhosttyApp{}
	local := g.parseGhosttyConfig("font-size = 14\ntheme = dark\n")
	remote := g.parseGhosttyConfig("font-size = 20\nwindow-padding = 5\n")

	merged := g.mergeGhosttyConfigs(local, remote)

	// Collect merged keys -> values.
	got := map[string]string{}
	for _, l := range merged {
		if l.key != "" {
			got[l.key] = l.value
		}
	}

	// Local font-size wins over remote.
	assert.Equal(t, "14", got["font-size"])
	// Local-only key preserved.
	assert.Equal(t, "dark", got["theme"])
	// Remote-only key appended.
	assert.Equal(t, "5", got["window-padding"])
}

func TestGhostty_formatGhosttyConfig_roundTrip(t *testing.T) {
	g := &GhosttyApp{}
	original := "# header comment\nfont-size = 14\n\ntheme = dark"
	lines := g.parseGhosttyConfig(original)
	formatted := g.formatGhosttyConfig(lines)

	// Comments/blank lines preserved verbatim; key=value normalized with spaces.
	assert.Contains(t, formatted, "# header comment")
	assert.Contains(t, formatted, "font-size = 14")
	assert.Contains(t, formatted, "theme = dark")

	// Re-parsing the formatted output yields the same key/value pairs.
	reparsed := g.parseGhosttyConfig(formatted)
	got := map[string]string{}
	for _, l := range reparsed {
		if l.key != "" {
			got[l.key] = l.value
		}
	}
	assert.Equal(t, "14", got["font-size"])
	assert.Equal(t, "dark", got["theme"])
}

func TestGhostty_Save_mergesRemoteIntoLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	g := NewGhosttyApp()

	configPath := filepath.Join(home, ".config", "ghostty", "config")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte("font-size = 14\ntheme = dark\n"), 0644))

	// Remote brings a new key plus a conflicting font-size (local should win).
	files := map[string]string{
		"~/.config/ghostty/config": "font-size = 20\nwindow-padding = 8\n",
	}
	require.NoError(t, g.Save(context.Background(), files, false))

	merged, err := os.ReadFile(configPath)
	require.NoError(t, err)
	s := string(merged)
	assert.Contains(t, s, "font-size = 14", "local font-size should win")
	assert.NotContains(t, s, "font-size = 20")
	assert.Contains(t, s, "theme = dark")
	assert.Contains(t, s, "window-padding = 8", "remote-only key appended")
}

func TestGhostty_Save_dryRunDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	g := NewGhosttyApp()

	configPath := filepath.Join(home, ".config", "ghostty", "config")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	original := "font-size = 14\n"
	require.NoError(t, os.WriteFile(configPath, []byte(original), 0644))

	files := map[string]string{
		"~/.config/ghostty/config": "window-padding = 8\n",
	}
	require.NoError(t, g.Save(context.Background(), files, true))

	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, original, string(after), "dry-run must not modify the file")
}

func TestGhostty_Save_newFileCreatesFromRemote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	g := NewGhosttyApp()

	configPath := filepath.Join(home, ".config", "ghostty", "config")
	files := map[string]string{
		"~/.config/ghostty/config": "font-size = 16\ntheme = light\n",
	}
	require.NoError(t, g.Save(context.Background(), files, false))

	written, err := os.ReadFile(configPath)
	require.NoError(t, err)
	s := string(written)
	assert.Contains(t, s, "font-size = 16")
	assert.Contains(t, s, "theme = light")
}

func TestGhostty_Save_identicalContentNoChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	g := NewGhosttyApp()

	configPath := filepath.Join(home, ".config", "ghostty", "config")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	// Content already in normalized form so merged == local.
	content := "font-size = 14"
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))
	info, err := os.Stat(configPath)
	require.NoError(t, err)

	files := map[string]string{"~/.config/ghostty/config": "font-size = 14"}
	require.NoError(t, g.Save(context.Background(), files, false))

	info2, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, info.ModTime(), info2.ModTime(), "no rewrite when merged content equals local")
}
