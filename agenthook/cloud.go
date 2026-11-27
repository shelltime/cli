package agenthook

import (
	"os/user"
	"strconv"
	"strings"
)

// Cloud signals, the values of Event.Cloud: what showed that a hook ran in a coding agent's
// cloud VM (a Cursor Cloud Agent, the Antigravity agent in a Gemini API sandbox, a
// claude.ai/code environment) rather than on one of the user's machines.
//
// Neither Cursor nor Antigravity marks a cloud run in a way every hook sees. Cursor names it
// only in sessionStart's is_background_agent, and its CURSOR_CODE_REMOTE is set in any remote
// workspace, an SSH host of the user's included, so it is kept as metadata, never as a cloud
// signal. Antigravity documents none; its Gemini API sandbox runs as the OS user antigravity.
// SHELLTIME_CLOUD=true in the cloud environment's variables (or secrets) marks every hook that
// runs there.
const (
	// CloudSignalCursorBackgroundAgent is Cursor's is_background_agent: Cloud Agents were
	// called Background Agents.
	CloudSignalCursorBackgroundAgent = "cursor_background_agent"
	// CloudSignalAntigravitySandbox is an Antigravity hook run by the OS user antigravity, the
	// user the Antigravity agent's Gemini API sandbox runs as.
	CloudSignalAntigravitySandbox = "antigravity_sandbox"
	// CloudSignalEnv is SHELLTIME_CLOUD=true in the hook's environment.
	CloudSignalEnv = "shelltime_cloud"
	// CloudSignalClaudeCodeRemote is CLAUDE_CODE_REMOTE=true, set in a Claude Code cloud
	// environment and never on a local machine.
	CloudSignalClaudeCodeRemote = "claude_code_remote"
)

// CloudEnvVar marks a machine as a cloud VM when it is true, for every agent.
const CloudEnvVar = "SHELLTIME_CLOUD"

// AntigravitySandboxUser is the OS user the Antigravity agent's Gemini API sandbox runs as. It
// isn't documented; it is what the sandbox shows.
const AntigravitySandboxUser = "antigravity"

// lookupOSUser returns the name of the OS user this process runs as; replaced in tests.
var lookupOSUser = func() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

const (
	claudeCodeRemoteEnvVar = "CLAUDE_CODE_REMOTE"
	// cursorCodeRemoteEnvVar is "true" for Cursor hooks that run in a remote workspace.
	cursorCodeRemoteEnvVar = "CURSOR_CODE_REMOTE"
)

// CloudEnvSignal returns the signal the environment gives that this process runs in a cloud
// VM, or "": SHELLTIME_CLOUD set to true, or a Claude Code cloud environment's
// CLAUDE_CODE_REMOTE=true. A nil getenv reads nothing.
func CloudEnvSignal(getenv func(string) string) string {
	if getenv == nil {
		return ""
	}
	if on, err := strconv.ParseBool(strings.TrimSpace(getenv(CloudEnvVar))); err == nil && on {
		return CloudSignalEnv
	}
	if getenv(claudeCodeRemoteEnvVar) == "true" {
		return CloudSignalClaudeCodeRemote
	}
	return ""
}

// InAntigravitySandbox reports whether this process runs as AntigravitySandboxUser. The user
// comes from the user database, else USER or LOGNAME: a container's user often has no entry in
// it. A nil getenv reads nothing.
func InAntigravitySandbox(getenv func(string) string) bool {
	name, err := lookupOSUser()
	if (err != nil || name == "") && getenv != nil {
		if name = getenv("USER"); name == "" {
			name = getenv("LOGNAME")
		}
	}
	return name == AntigravitySandboxUser
}

// cloudSignal returns what shows that the hook behind p ran in a cloud VM, or "": what the
// agent says first, then the environment.
func cloudSignal(agent string, p payload, getenv func(string) string) string {
	switch agent {
	case AgentCursor:
		if background, ok := p.boolean("is_background_agent"); ok && background {
			return CloudSignalCursorBackgroundAgent
		}
	case AgentAntigravity:
		if InAntigravitySandbox(getenv) {
			return CloudSignalAntigravitySandbox
		}
	}
	return CloudEnvSignal(getenv)
}
