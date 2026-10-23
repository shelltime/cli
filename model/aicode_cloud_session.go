package model

import (
	"net/http"
	"strings"
)

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

// IsAICloudOnly reports whether this machine collects only AI coding agent telemetry, and no
// shell commands, editor heartbeats, aliases, dotfiles or anything else that would make it a
// host. cfg.AICloudOnly decides when it is set; unset, the mode is on in a Claude Code cloud
// environment, a throwaway VM that isn't one of the user's machines.
func IsAICloudOnly(cfg ShellTimeConfig, getenv func(string) string) bool {
	if cfg.AICloudOnly != nil {
		return *cfg.AICloudOnly
	}
	return getenv("CLAUDE_CODE_REMOTE") == "true"
}

// ProxyInjectedToken is the token a Claude Code cloud environment's sessions sign in with when
// the real one is a network secret: the agent proxy adds `Authorization: CLI <token>` to every
// request for api.shelltime.xyz after it leaves the VM, so the token is never on disk or in the
// environment. Cloud sessions read GITHUB_TOKEN as the same placeholder for the GitHub proxy.
const ProxyInjectedToken = "proxy-injected"

// setCLIAuthorization authenticates a request to the ShellTime API. With ProxyInjectedToken it
// sends no Authorization header, so the one the proxy adds is the only one the server reads.
func setCLIAuthorization(req *http.Request, token string) {
	if token == ProxyInjectedToken {
		return
	}
	req.Header.Set("Authorization", "CLI "+token)
}
