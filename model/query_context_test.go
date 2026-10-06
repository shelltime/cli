package model

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const queryContextFixtures = "../fixtures/query_context"

func readQueryFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(queryContextFixtures, name))
	require.NoError(t, err)
	return string(b)
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

func TestSanitizeContextString(t *testing.T) {
	esc := string(rune(0x1b))
	tests := []struct {
		name     string
		in       string
		maxRunes int
		want     string
	}{
		{"empty", "", 10, ""},
		{"collapses whitespace", "  feat:\tadd\n\nthing  ", 50, "feat: add thing"},
		{"strips CSI color codes", esc + "[31mred" + esc + "[0m text", 50, "red text"},
		{"strips OSC title", esc + "]0;title" + string(rune(7)) + "after", 50, "after"},
		{"strips control chars", "a" + string(rune(0)) + "b", 50, "a b"},
		{"truncates by rune", "日本語のテキスト", 3, "日本語…"},
		{"invalid utf8", "ok\xffok", 10, "okok"},
		{"exact length kept", "abc", 3, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SanitizeContextString(tt.in, tt.maxRunes))
		})
	}
}

func TestParseMakeTargets(t *testing.T) {
	got := ParseMakeTargets(readQueryFixture(t, "Makefile"))
	// Assignments, .PHONY/.DEFAULT_GOAL, %.o pattern rules, $(BIN) and
	// bin/tool are skipped; "test lint:" yields both; common names lead.
	assert.Equal(t, []string{"build", "test", "lint", "clean", "release", "deps"}, got)
}

func TestParseJustRecipes(t *testing.T) {
	got := ParseJustRecipes(readQueryFixture(t, "justfile"))
	// set/alias/assignments/export/import/attributes and _private recipes
	// are skipped; "@test *args:" and parameterized recipes are kept.
	assert.Equal(t, []string{"build", "test", "default", "release-notes"}, got)
}

func TestReadPackageJSON(t *testing.T) {
	scripts, manager := readPackageJSON(filepath.Join(queryContextFixtures, "package.json"))
	assert.Equal(t, "pnpm", manager)
	assert.Equal(t, []string{"dev", "build", "test", "lint", "codegen", "zeta"}, scripts)

	scripts, manager = readPackageJSON(filepath.Join(t.TempDir(), "missing.json"))
	assert.Nil(t, scripts)
	assert.Empty(t, manager)
}

func TestPrioritizeScriptsCapsAt20(t *testing.T) {
	var names []string
	for _, r := range "abcdefghijklmnopqrstuvwxyz" {
		names = append(names, "script-"+string(r))
	}
	names = append(names, "test", "test")
	got := prioritizeScripts(names)
	require.Len(t, got, queryScriptsMax)
	assert.Equal(t, "test", got[0])
	assert.Equal(t, "script-a", got[1])
}

func TestDetectProjectMonorepo(t *testing.T) {
	outer := t.TempDir()
	root := filepath.Join(outer, "repo")
	writeFiles(t, outer, map[string]string{
		// Manifests above the repository root must not leak in
		"go.mod":       "module outer\n",
		"package.json": `{"scripts":{"leak":"x"}}`,
	})
	writeFiles(t, root, map[string]string{
		".git/HEAD":                 "ref: refs/heads/main\n",
		"pnpm-lock.yaml":            "lockfileVersion: '9.0'\n",
		"package.json":              `{"name":"root","scripts":{"build":"turbo build"}}`,
		"Makefile":                  readQueryFixture(t, "Makefile"),
		"packages/web/package.json": `{"name":"web","scripts":{"dev":"next dev","test":"vitest"}}`,
		"packages/web/src/index.ts": "export {}\n",
		"packages/web/Dockerfile":   "FROM node:22\n",
	})

	p := DetectProject(filepath.Join(root, "packages", "web"), "")
	require.NotNil(t, p)
	assert.Equal(t, []string{"dev", "test"}, p.PackageScripts, "nearest package.json wins")
	assert.Equal(t, []string{"pnpm"}, p.PackageManagers, "workspace lockfile is found by walking up")
	assert.Equal(t, []string{"node", "docker", "make"}, p.Types)
	assert.Equal(t, []string{"build", "test", "lint", "clean", "release", "deps"}, p.MakeTargets)
}

func TestDetectProjectPackageManagerField(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".git/HEAD":    "ref: refs/heads/main\n",
		"package.json": `{"packageManager":"yarn@4.1.0","scripts":{"start":"node ."}}`,
		// The packageManager field beats a stray lockfile
		"package-lock.json": "{}",
		"uv.lock":           "",
		"pyproject.toml":    "[project]\nname='x'\n",
	})
	p := DetectProject(root, "")
	require.NotNil(t, p)
	assert.Equal(t, []string{"yarn", "uv"}, p.PackageManagers)
	assert.Equal(t, []string{"node", "python"}, p.Types)
}

func TestDetectProjectStopsAtHomeAndEmpty(t *testing.T) {
	home := t.TempDir()
	writeFiles(t, home, map[string]string{"package.json": `{"scripts":{"x":"y"}}`})
	sub := filepath.Join(home, "notes")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	assert.Nil(t, DetectProject(sub, home), "home's own manifests are ignored")
	assert.Nil(t, DetectProject(home, home))
	assert.Nil(t, DetectProject("", home))
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"b.txt":        "",
		"a.go":         "",
		".env.example": "",
		".DS_Store":    "",
		"src/main.go":  "",
		".git/HEAD":    "",
		"weird\nname":  "",
	})

	d := ListDir(dir)
	require.NotNil(t, d)
	assert.Equal(t, []string{".env.example", "a.go", "b.txt", "src/", "weird name"}, d.Entries)
	assert.False(t, d.Truncated)

	assert.Nil(t, ListDir(filepath.Join(dir, "missing")))
	assert.Nil(t, ListDir(t.TempDir()), "empty directory")
}

func TestListDirTruncates(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{}
	for i := 0; i < queryDirMaxEntries+5; i++ {
		files[fmt.Sprintf("f%02d", i)] = ""
	}
	writeFiles(t, dir, files)

	d := ListDir(dir)
	require.NotNil(t, d)
	assert.Len(t, d.Entries, queryDirMaxEntries)
	assert.Equal(t, "f00", d.Entries[0])
	assert.True(t, d.Truncated)
}
