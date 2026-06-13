package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetchClaudeCodeOAuthToken_LinuxDispatch covers the runtime.GOOS=="linux"
// branch of the dispatcher, which delegates to fetchOAuthTokenFromCredentialsFile.
func TestFetchClaudeCodeOAuthToken_LinuxDispatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux credentials-file dispatch path")
	}

	t.Run("valid credentials file", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		claudeDir := filepath.Join(home, ".claude")
		require.NoError(t, os.MkdirAll(claudeDir, 0o700))
		content := `{"claudeAiOauth":{"accessToken":"sk-dispatch-token"}}`
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(content), 0o600))

		tok, err := fetchClaudeCodeOAuthToken()
		require.NoError(t, err)
		assert.Equal(t, "sk-dispatch-token", tok)
	})

	t.Run("missing credentials file", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		tok, err := fetchClaudeCodeOAuthToken()
		require.Error(t, err)
		assert.Empty(t, tok)
		assert.Contains(t, err.Error(), "credentials file read failed")
	})
}

// TestShortenAPIError_Boundaries exercises the exact "failed"-prefix boundary
// and a short non-matching message in shortenAPIError (Anthropic variant).
func TestShortenAPIError_Boundaries(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want string
	}{
		{"status 500", fmt.Errorf("anthropic usage API returned status %d", 500), "api:500"},
		{"decode prefix", fmt.Errorf("failed to decode usage response: x"), "api:decode"},
		{"short message", fmt.Errorf("oops"), "network"},
		{"empty-ish", fmt.Errorf(" "), "network"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, shortenAPIError(c.in))
		})
	}
}
