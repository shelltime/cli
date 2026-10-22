package daemon

import (
	"os"
	"testing"
)

// TestMain clears the Claude Code cloud session environment, so the suite gives the same results
// inside a cloud session (where CLAUDE_CODE_REMOTE=true) as on CI.
func TestMain(m *testing.M) {
	os.Unsetenv("CLAUDE_CODE_REMOTE")
	os.Unsetenv("CLAUDE_CODE_REMOTE_SESSION_ID")
	os.Exit(m.Run())
}
