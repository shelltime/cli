package daemon

import (
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withRunningProcesses stubs the process list for the duration of the test.
func withRunningProcesses(t *testing.T, names ...string) {
	t.Helper()
	orig := listProcessNamesFunc
	listProcessNamesFunc = func() ([]string, error) { return names, nil }
	t.Cleanup(func() { listProcessNamesFunc = orig })
}

func TestIsProcessRunning(t *testing.T) {
	tests := []struct {
		name      string
		processes []string
		claude    bool
		codex     bool
	}{
		{
			name:      "macOS Claude Code binary",
			processes: []string{"/Users/me/.local/bin/claude"},
			claude:    true,
		},
		{
			name:      "macOS Claude desktop app",
			processes: []string{"/Applications/Claude.app/Contents/MacOS/Claude"},
			claude:    true,
		},
		{
			name:      "macOS ChatGPT app",
			processes: []string{"/Applications/ChatGPT.app/Contents/MacOS/ChatGPT"},
			codex:     true,
		},
		{
			name:      "macOS Codex app",
			processes: []string{"/Applications/Codex.app/Contents/MacOS/Codex"},
			codex:     true,
		},
		{
			name:      "linux short names",
			processes: []string{"bash", "claude", "codex"},
			claude:    true,
			codex:     true,
		},
		{
			name:      "only the basename is matched",
			processes: []string{"/Users/me/.codex/plugins/node", "/Users/me/.claude/hooks/python3"},
		},
		{
			name:      "unrelated processes",
			processes: []string{"/sbin/launchd", "/bin/zsh", "shelltime-daemon"},
		},
		{
			name: "no processes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withRunningProcesses(t, tc.processes...)
			assert.Equal(t, tc.claude, isClaudeRunning(), "claude")
			assert.Equal(t, tc.codex, isCodexRunning(), "codex")
		})
	}
}

func TestIsProcessRunning_ListErrorAssumesRunning(t *testing.T) {
	orig := listProcessNamesFunc
	listProcessNamesFunc = func() ([]string, error) { return nil, errors.New("ps not found") }
	t.Cleanup(func() { listProcessNamesFunc = orig })

	assert.True(t, isClaudeRunning())
	assert.True(t, isCodexRunning())
}

func TestListProcessNames(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("ps is only used on darwin/linux")
	}

	names, err := listProcessNames()
	require.NoError(t, err)
	assert.NotEmpty(t, names, "the test process itself should be listed")
	for _, name := range names {
		assert.NotEmpty(t, name)
	}
}

func TestAnthropicUsageCacheTTLShorterThanSyncInterval(t *testing.T) {
	// A TTL equal to the sync interval makes every other tick land just before it expires.
	assert.Less(t, anthropicUsageCacheTTL, usageSyncInterval)
}
