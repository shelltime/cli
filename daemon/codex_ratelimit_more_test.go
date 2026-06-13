package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCodexPathExists_NonNotExistError covers the third return branch of
// codexPathExists: a stat error that is NOT os.ErrNotExist (here ENOTDIR,
// produced by treating a regular file as a path component) must surface as
// (false, err).
func TestCodexPathExists_NonNotExistError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	// Stat-ing "afile/child" yields ENOTDIR, which is neither nil nor ErrNotExist.
	ok, err := codexPathExists(filepath.Join(file, "child"))
	require.Error(t, err)
	assert.False(t, ok)
	assert.False(t, errors.Is(err, os.ErrNotExist))
}

// TestCodexConfigAndAuthPaths verifies the happy path of codexConfigDirPath and
// codexAuthFilePath against a controlled HOME.
func TestCodexConfigAndAuthPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := codexConfigDirPath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".codex"), dir)

	authPath, err := codexAuthFilePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".codex", "auth.json"), authPath)
}

// TestCodexInstallationStatus_DirStatError covers the branch where the path
// existence check returns a hard error (not just "not found") for the .codex
// directory: codexInstallationStatus must propagate it.
func TestCodexInstallationStatus_DirStatError(t *testing.T) {
	orig := codexPathExistsFunc
	t.Cleanup(func() { codexPathExistsFunc = orig })

	sentinel := errors.New("stat boom")
	codexPathExistsFunc = func(path string) (bool, error) {
		return false, sentinel
	}

	ok, err := codexInstallationStatus()
	assert.False(t, ok)
	assert.ErrorIs(t, err, sentinel)
}

// TestCodexInstallationStatus_AuthStatError covers the branch where the .codex
// dir exists but the auth-file existence check returns a hard error.
func TestCodexInstallationStatus_AuthStatError(t *testing.T) {
	orig := codexPathExistsFunc
	t.Cleanup(func() { codexPathExistsFunc = orig })

	sentinel := errors.New("auth stat boom")
	calls := 0
	codexPathExistsFunc = func(path string) (bool, error) {
		calls++
		if calls == 1 {
			return true, nil // .codex dir present
		}
		return false, sentinel // auth.json stat errors
	}

	ok, err := codexInstallationStatus()
	assert.False(t, ok)
	assert.ErrorIs(t, err, sentinel)
}

// TestLoadCodexAuth_OnlyOpenAIKey confirms that an auth.json with only the API
// key (nil tokens) is reported as invalid auth (errCodexAuthInvalid), and that
// a populated account_id round-trips when tokens are present.
func TestLoadCodexAuth_AccountIDRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0o700))

	content := `{"tokens":{"access_token":"acc","account_id":"acct-xyz"}}`
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(content), 0o600))

	auth, err := loadCodexAuth()
	require.NoError(t, err)
	assert.Equal(t, "acc", auth.AccessToken)
	assert.Equal(t, "acct-xyz", auth.AccountID)
}

// TestMapWhamWindow_ZeroWindow ensures division/seconds->minutes handling for a
// sub-minute window (LimitWindowSeconds < 60 -> 0 minutes) and a secondary
// position label.
func TestMapWhamWindow_SubMinuteSecondary(t *testing.T) {
	w := &whamRateLimitWindow{
		UsedPercent:        5,
		LimitWindowSeconds: 30, // < 60 -> 0 minutes
		ResetAt:            999,
	}
	got := mapWhamWindow("code_review_rate_limit", "secondary", w)
	assert.Equal(t, "code_review_rate_limit:secondary", got.LimitID)
	assert.Equal(t, float64(5), got.UsagePercentage)
	assert.Equal(t, int64(999), got.ResetAt)
	assert.Equal(t, 0, got.WindowDurationMinutes)
}

// TestShortenCodexAPIError_Boundaries adds boundary cases for the codex error
// shortener not already covered (status with different code, exact "failed"
// prefix, and a generic network fallback).
func TestShortenCodexAPIError_Boundaries(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want string
	}{
		{"status 503", errors.New("codex usage API returned status 503"), "api:503"},
		{"failed prefix decode", errors.New("failed to decode codex usage response: EOF"), "api:decode"},
		{"generic", errors.New("some other thing"), "network"},
		{"dir missing maps to network", errCodexDirMissing, "network"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, shortenCodexAPIError(c.in))
		})
	}
}
