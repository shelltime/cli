package model

import "strings"

// AICodeCloudSessionIDAttribute is the standard attribute Claude Code puts on every event and
// metric of a cloud session (claude.ai/code): its CLAUDE_CODE_REMOTE_SESSION_ID, like cse_01AbC.
// The server marks sessions that carry it as cloud sessions.
const AICodeCloudSessionIDAttribute = "ccr.session.id"

// CloudSessionIDFromEnv returns the Claude Code cloud session the process runs in, or "". A
// cloud VM sets CLAUDE_CODE_REMOTE=true (never true on a local machine) and
// CLAUDE_CODE_REMOTE_SESSION_ID; the daemon inherits both from the SessionStart hook that
// starts it there.
func CloudSessionIDFromEnv(getenv func(string) string) string {
	if getenv("CLAUDE_CODE_REMOTE") != "true" {
		return ""
	}
	return strings.TrimSpace(getenv("CLAUDE_CODE_REMOTE_SESSION_ID"))
}
