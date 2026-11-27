package model

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malamtime/cli/agenthook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAntigravityHooks(t *testing.T) (*AntigravityHooksService, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := NewAntigravityHooksService()
	require.Equal(t, filepath.Join(home, ".gemini", "config", "hooks.json"), svc.Path())
	return svc, svc.Path()
}

func readAntigravityGroup(t *testing.T, path string) map[string][]map[string]any {
	t.Helper()
	data := mustRead(t, path)
	require.False(t, bytes.HasPrefix(data, utf8BOM), "never written with a BOM")
	require.True(t, json.Valid(data), "strict JSON")
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &root))
	var group map[string][]map[string]any
	require.NoError(t, json.Unmarshal(root[AntigravityHookGroup], &group))
	return group
}

func assertAntigravityGroup(t *testing.T, path, bin string) {
	t.Helper()
	group := readAntigravityGroup(t, path)
	assert.Len(t, group, len(agenthook.AntigravityEvents))
	assert.NotContains(t, group, "PreToolUse", "PreToolUse is never registered")
	handler := func(event string) map[string]any {
		return map[string]any{"type": "command", "command": AntigravityHookCommand(bin, event), "timeout": float64(15)}
	}
	for _, event := range []string{"SessionStart", "PreInvocation", "PostInvocation", "Stop"} {
		assert.Equal(t, []map[string]any{handler(event)}, group[event], event)
	}
	assert.Equal(t, []map[string]any{{"matcher": "*", "hooks": []any{handler("PostToolUse")}}}, group["PostToolUse"])
}

func TestAntigravityHooks_InstallCreatesFile(t *testing.T) {
	svc, path := newTestAntigravityHooks(t)

	require.NoError(t, svc.Install(testHookBin))

	assertAntigravityGroup(t, path, testHookBin)
	text := string(mustRead(t, path))
	assert.Contains(t, text, `"command": "/usr/local/bin/shelltime hook-ingest --agent agy --event Stop"`)
	assert.Less(t, strings.Index(text, `"SessionStart"`), strings.Index(text, `"PreInvocation"`))
	assert.Less(t, strings.Index(text, `"PostInvocation"`), strings.Index(text, `"Stop"`))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestAntigravityHooks_InstallIntoEmptyAndBareFiles(t *testing.T) {
	for _, content := range []string{"", "\n", "{}"} {
		t.Run(content, func(t *testing.T) {
			svc, path := newTestAntigravityHooks(t)
			writeFile(t, path, content, 0644)
			require.NoError(t, svc.Install(testHookBin))
			assertAntigravityGroup(t, path, testHookBin)
		})
	}
}

func TestAntigravityHooks_InstallKeepsOtherGroups(t *testing.T) {
	svc, path := newTestAntigravityHooks(t)
	writeFile(t, path, `{
  "model-armor": {"PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "armor check", "timeout": 5}]}]},
  "zz-last": {"Stop": [{"type": "command", "command": "notify"}]}
}`, 0600)

	require.NoError(t, svc.Install(testHookBin))

	assertAntigravityGroup(t, path, testHookBin)
	text := string(mustRead(t, path))
	assert.Less(t, strings.Index(text, `"model-armor"`), strings.Index(text, `"zz-last"`), "groups keep their order")
	assert.Less(t, strings.Index(text, `"zz-last"`), strings.Index(text, `"shelltime"`), "our group is added last")
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(text), &root))
	assert.JSONEq(t, `{"PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "armor check", "timeout": 5}]}]}`, string(root["model-armor"]))
	assert.JSONEq(t, `{"Stop": [{"type": "command", "command": "notify"}]}`, string(root["zz-last"]))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestAntigravityHooks_InstallIsIdempotentAndUpdatesTheBinary(t *testing.T) {
	svc, path := newTestAntigravityHooks(t)
	writeFile(t, path, `{"a":{"Stop":[]},"shelltime":{"PreToolUse":[{"matcher":"*","hooks":[]}]},"b":{}}`, 0644)

	require.NoError(t, svc.Install(testHookBin))
	first := mustRead(t, path)
	require.NoError(t, svc.Install(testHookBin))
	assert.Equal(t, string(first), string(mustRead(t, path)))

	text := string(first)
	assert.Less(t, strings.Index(text, `"a"`), strings.Index(text, `"shelltime"`))
	assert.Less(t, strings.Index(text, `"shelltime"`), strings.Index(text, `"b"`), "our group is replaced in place")
	assertAntigravityGroup(t, path, testHookBin)

	newBin := "/opt/homebrew/bin/shelltime"
	require.NoError(t, svc.Install(newBin))
	assertAntigravityGroup(t, path, newBin)
	assert.NotContains(t, string(mustRead(t, path)), testHookBin)
}

func TestAntigravityHooks_Uninstall(t *testing.T) {
	svc, path := newTestAntigravityHooks(t)
	writeFile(t, path, `{"mine":{"Stop":[{"type":"command","command":"notify","timeout":1}]}}`, 0644)
	require.NoError(t, svc.Install(testHookBin))

	require.NoError(t, svc.Uninstall())

	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(mustRead(t, path), &root))
	assert.NotContains(t, root, AntigravityHookGroup)
	assert.JSONEq(t, `{"Stop":[{"type":"command","command":"notify","timeout":1}]}`, string(root["mine"]))

	before := mustRead(t, path)
	require.NoError(t, svc.Uninstall())
	assert.Equal(t, before, mustRead(t, path), "nothing of ours left: untouched")
}

func TestAntigravityHooks_UninstallMissingFile(t *testing.T) {
	svc, path := newTestAntigravityHooks(t)
	require.NoError(t, svc.Uninstall())
	assert.NoFileExists(t, path)
}

func TestAntigravityHooks_RefusesInvalidFiles(t *testing.T) {
	for name, content := range map[string]string{
		"invalid JSON":   `{"a":`,
		"BOM":            "\xEF\xBB\xBF{}",
		"line comment":   "{\n  // mine\n  \"a\": {}\n}",
		"block comment":  `{/* x */"a":{}}`,
		"trailing comma": `{"a":{},}`,
		"not an object":  `["shelltime"]`,
	} {
		t.Run(name, func(t *testing.T) {
			svc, path := newTestAntigravityHooks(t)
			writeFile(t, path, content, 0640)

			assert.ErrorIs(t, svc.Install(testHookBin), ErrHookFileInvalid)
			assert.ErrorIs(t, svc.Uninstall(), ErrHookFileInvalid)
			_, err := svc.Status()
			assert.ErrorIs(t, err, ErrHookFileInvalid)

			assert.Equal(t, content, string(mustRead(t, path)), "never rewritten")
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0640), info.Mode().Perm())
		})
	}
}

func TestAntigravityHooks_Status(t *testing.T) {
	svc, path := newTestAntigravityHooks(t)

	status, err := svc.Status()
	require.NoError(t, err)
	assert.False(t, status.Installed)
	assert.Equal(t, agenthook.AntigravityEvents, status.MissingEvents)

	bin := "/Applications/My Tools/shelltime"
	require.NoError(t, svc.Install(bin))
	status, err = svc.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Empty(t, status.MissingEvents)
	assert.Equal(t, []string{bin}, status.Binaries)

	writeFile(t, path, `{"shelltime":{"Stop":[{"type":"command","command":"/x/shelltime hook-ingest --agent agy --event Stop","timeout":15}]}}`, 0644)
	status, err = svc.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, []string{"SessionStart", "PreInvocation", "PostToolUse", "PostInvocation"}, status.MissingEvents)
	assert.Equal(t, []string{"/x/shelltime"}, status.Binaries)
}
