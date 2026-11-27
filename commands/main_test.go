package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/agenthook"
)

// TestMain keeps the suite away from the ShellTime daemon of the machine it runs on: `track`
// hands commands to the daemon on the default socket before reading any config, so a running
// daemon (yours, or a Claude Code cloud session's) would upload the tests' commands to its
// account. It also clears the cloud environment variables, so the suite gives the same results
// inside a cloud session (where CLAUDE_CODE_REMOTE=true, SHELLTIME_CLOUD=true or the OS user is
// antigravity) as on CI.
func TestMain(m *testing.M) {
	os.Unsetenv("CLAUDE_CODE_REMOTE")
	os.Unsetenv("CLAUDE_CODE_REMOTE_SESSION_ID")
	os.Unsetenv("SHELLTIME_CLOUD")
	// An Antigravity sandbox runs as the OS user antigravity, which counts as a cloud VM
	agenthook.OSUserLookup = func() (string, error) { return "shelltime-test", nil }
	trackSocketPath = filepath.Join(os.TempDir(), fmt.Sprintf("shelltime-commands-test-%d.sock", os.Getpid()))
	// `shelltime q` counts calls for its tip under $HOME; keep the count out of the real one
	tipDir, err := os.MkdirTemp("", "shelltime-commands-test-tips")
	if err != nil {
		panic(err)
	}
	queryTipCountPath = func() string { return filepath.Join(tipDir, "query-tip.count") }
	code := m.Run()
	os.RemoveAll(tipDir)
	os.Exit(code)
}
