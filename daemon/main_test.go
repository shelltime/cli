package daemon

import (
	"fmt"
	"os"
	"testing"
)

// TestMain clears the Claude Code cloud session environment, so the suite gives the same results
// inside a cloud session (where CLAUDE_CODE_REMOTE=true) as on CI.
//
// It also gives the suite its own HOME. Handlers queue what they couldn't send under
// $HOME/.shelltime (sync-pending.jsonl, coding-heartbeat.data.log), and a daemon running on the
// machine (yours, or a Claude Code cloud session's) resends whatever it finds there to its
// account. A test that forgets to set HOME must not be able to leave anything in the real one.
func TestMain(m *testing.M) {
	os.Unsetenv("CLAUDE_CODE_REMOTE")
	os.Unsetenv("CLAUDE_CODE_REMOTE_SESSION_ID")
	os.Exit(runInTempHome(m))
}

func runInTempHome(m *testing.M) int {
	home, err := os.MkdirTemp("", "shelltime-daemon-test-home-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create a HOME for the tests:", err)
		return 1
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	return m.Run()
}
