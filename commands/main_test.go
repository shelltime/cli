package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain keeps the suite away from the ShellTime daemon of the machine it runs on: `track`
// hands commands to the daemon on the default socket before reading any config, so a running
// daemon (yours, or a Claude Code cloud session's) would upload the tests' commands to its
// account. It also clears the Claude Code cloud session environment, so the suite gives the same
// results inside a cloud session (where CLAUDE_CODE_REMOTE=true) as on CI.
func TestMain(m *testing.M) {
	os.Unsetenv("CLAUDE_CODE_REMOTE")
	os.Unsetenv("CLAUDE_CODE_REMOTE_SESSION_ID")
	trackSocketPath = filepath.Join(os.TempDir(), fmt.Sprintf("shelltime-commands-test-%d.sock", os.Getpid()))
	os.Exit(m.Run())
}
