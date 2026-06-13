package daemon

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchKnownName(t *testing.T) {
	testCases := []struct {
		name       string
		process    string
		knownNames []string
		expected   string
	}{
		// terminals
		{"exact terminal", "alacritty", knownTerminals, "alacritty"},
		{"case-insensitive", "ITerm2", knownTerminals, "iterm2"},
		// NOTE: knownTerminals lists "terminal" (macOS Terminal.app) before
		// "gnome-terminal", and matchKnownName returns the FIRST substring match
		// in list order. So "gnome-terminal-server" matches "terminal" first.
		// This documents the actual product behavior, not a bug fix.
		{"first-match-wins ordering quirk", "gnome-terminal-server", knownTerminals, "terminal"},
		{"konsole no false terminal prefix", "konsole", knownTerminals, "konsole"},
		{"vscode code match", "Code Helper", knownTerminals, "code"},
		{"cursor", "Cursor", knownTerminals, "cursor"},
		{"no terminal match", "bash", knownTerminals, ""},
		// multiplexers
		{"tmux", "tmux: server", knownMultiplexers, "tmux"},
		{"screen", "SCREEN", knownMultiplexers, "screen"},
		{"zellij", "zellij", knownMultiplexers, "zellij"},
		{"no mux match", "fish", knownMultiplexers, ""},
		// remote
		{"sshd", "sshd: user@pts/0", knownRemote, "sshd"},
		{"docker", "dockerd", knownRemote, "docker"},
		{"containerd", "containerd-shim", knownRemote, "containerd"},
		{"no remote match", "zsh", knownRemote, ""},
		// empty
		{"empty input", "", knownTerminals, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, matchKnownName(tc.process, tc.knownNames))
		})
	}
}

func TestResolveTerminal_NonPositivePPID(t *testing.T) {
	term, mux := ResolveTerminal(0)
	assert.Equal(t, "", term)
	assert.Equal(t, "", mux)

	term, mux = ResolveTerminal(-5)
	assert.Equal(t, "", term)
	assert.Equal(t, "", mux)
}

func TestResolveTerminal_PID1(t *testing.T) {
	// ppid==1 stops the walk immediately (currentPID <= 1), so neither terminal
	// nor multiplexer is found -> returns the "unknown" sentinel.
	term, mux := ResolveTerminal(1)
	assert.Equal(t, "unknown", term)
	assert.Equal(t, "", mux)
}

func TestResolveTerminal_CurrentProcessNoPanic(t *testing.T) {
	// Walking up from the current process must not panic and returns strings.
	// The exact value is environment-dependent (test runner / CI), so we only
	// assert it terminates and produces a defined result.
	assert.NotPanics(t, func() {
		term, mux := ResolveTerminal(os.Getpid())
		// terminal is either a known name, "unknown", or "" (if a terminal was
		// never found but a multiplexer was). Just ensure no panic / it returns.
		_ = term
		_ = mux
	})
}

func TestGetProcessName_InvalidPID(t *testing.T) {
	// A PID that almost certainly does not exist yields an empty name on both
	// linux (/proc miss) and darwin (ps error).
	assert.Equal(t, "", getProcessName(2147483646))
}

func TestGetParentPID_InvalidPID(t *testing.T) {
	assert.Equal(t, 0, getParentPID(2147483646))
}
