package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaskSensitiveTokens_ShortToken exercises the maskToken len<=8 branch: a
// short JWT-shaped token is fully masked with asterisks (no head/tail kept).
func TestMaskSensitiveTokens_ShortToken(t *testing.T) {
	// "ey" + a.b.c each <= a few chars so the whole match is <= 8 chars.
	out := MaskSensitiveTokens("eyA.b.c")
	// The matched token "eyA.b.c" is 7 chars -> fully replaced by 7 asterisks,
	// then the >=4 asterisk collapse reduces runs of 4+ to exactly 3.
	assert.Equal(t, "***", out)
	assert.NotContains(t, out, "eyA")
}

// TestSendDotfilesToServer_Empty covers the no-dotfiles short-circuit.
func TestSendDotfilesToServer_Empty(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	userID, err := SendDotfilesToServer(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, userID)
	assert.False(t, called, "no request for empty dotfiles")
}

// TestSendDotfilesToServer_HappyWithErrorResult covers the success path that
// also iterates results and logs per-item errors, returning the userId.
func TestSendDotfilesToServer_HappyWithErrorResult(t *testing.T) {
	var gotPath string
	var body struct {
		Dotfiles []DotfileItem `json:"dotfiles"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":1,"failed":1,"userId":77,"results":[{"app":"git","path":"~/.gitconfig","status":"error","error":"conflict"}]}`))
	}))
	defer server.Close()

	items := []DotfileItem{{App: "git", Path: "~/.gitconfig", Content: "x", FileType: "file"}}
	userID, err := SendDotfilesToServer(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL}, items)
	require.NoError(t, err)
	assert.Equal(t, 77, userID)
	assert.Equal(t, "/api/v1/dotfiles/push", gotPath)
	require.Len(t, body.Dotfiles, 1)
	// Hostname is auto-populated when empty.
	assert.NotEmpty(t, body.Dotfiles[0].Hostname)
}

// TestSendDotfilesToServer_ErrorPath covers the wrapped-error branch.
func TestSendDotfilesToServer_ErrorPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"rejected"}`))
	}))
	defer server.Close()

	items := []DotfileItem{{App: "git", Path: "~/.gitconfig", Content: "x", Hostname: "h"}}
	_, err := SendDotfilesToServer(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL}, items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send dotfiles to server")
	assert.Contains(t, err.Error(), "rejected")
}

// TestAICodeOtelBase_AddEnvLines_MissingFileErrors covers the open-error branch
// in addEnvLines (called on a path that does not exist).
func TestAICodeOtelBase_AddEnvLines_MissingFileErrors(t *testing.T) {
	b := &baseAICodeOtelEnvService{}
	err := b.addEnvLines(filepath.Join(t.TempDir(), "nope"), []string{"export A=1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open file")
}

// TestAICodeOtelBase_RemoveEnvLines_MissingFileErrors covers the open-error
// branch in removeEnvLines.
func TestAICodeOtelBase_RemoveEnvLines_MissingFileErrors(t *testing.T) {
	b := &baseAICodeOtelEnvService{}
	err := b.removeEnvLines(filepath.Join(t.TempDir(), "nope"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open file")
}

// TestAICodeOtelBase_CheckEnvLines_MissingFileErrors covers the read-error
// branch in checkEnvLines.
func TestAICodeOtelBase_CheckEnvLines_MissingFileErrors(t *testing.T) {
	b := &baseAICodeOtelEnvService{}
	ok, err := b.checkEnvLines(filepath.Join(t.TempDir(), "nope"))
	require.Error(t, err)
	assert.False(t, ok)
}

// TestAICodeOtelBase_RemoveEnvLines_StripsBlock writes a file that already
// contains a marker block plus user content, then removes it and asserts only
// the block is gone (covers the in-block scanning branches of removeEnvLines).
func TestAICodeOtelBase_RemoveEnvLines_StripsBlock(t *testing.T) {
	b := &baseAICodeOtelEnvService{}
	dir := t.TempDir()
	path := filepath.Join(dir, "rc")
	content := "keep1\n" + aiCodeOtelMarkerStart + "\nexport X=1\n" + aiCodeOtelMarkerEnd + "\nkeep2\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	require.NoError(t, b.removeEnvLines(path))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(got)
	assert.Contains(t, s, "keep1")
	assert.Contains(t, s, "keep2")
	assert.NotContains(t, s, "export X=1")
	assert.NotContains(t, s, aiCodeOtelMarkerStart)
}

// TestCodexOtelConfig_InstallParseError covers Install's "parse existing config"
// error branch when ~/.codex/config.toml already holds malformed TOML.
func TestCodexOtelConfig_InstallParseError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte("this is = = bad ]["), 0o644))

	err := NewCodexOtelConfigService().Install()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse existing config")
}

// TestCodexOtelConfig_UninstallParseError covers Uninstall's parse-error branch.
func TestCodexOtelConfig_UninstallParseError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte("bad = = ]["), 0o644))

	err := NewCodexOtelConfigService().Uninstall()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config")
}

// TestZshAICodeOtelEnv_UninstallRemovesBlock covers the zsh Uninstall path on an
// existing file with an installed block.
func TestZshAICodeOtelEnv_UninstallRemovesBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	zshrc := filepath.Join(home, ".zshrc")
	require.NoError(t, os.WriteFile(zshrc, []byte("export USERVAR=1\n"), 0o644))

	svc := NewZshAICodeOtelEnvService()
	require.NoError(t, svc.Install())
	require.NoError(t, svc.Check())

	require.NoError(t, svc.Uninstall())
	require.Error(t, svc.Check(), "block removed -> Check fails")

	got, err := os.ReadFile(zshrc)
	require.NoError(t, err)
	assert.Contains(t, string(got), "export USERVAR=1")
	assert.NotContains(t, string(got), "CLAUDE_CODE_ENABLE_TELEMETRY")
}
