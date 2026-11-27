package agenthook

import "strings"

// Antigravity's hook events (a project's .agents/hooks.json), as registered by
// `shelltime agy install`. PreToolUse is left out on purpose: a reply without a decision
// denies the tool call, and an "allow" may approve calls the user would have been asked about.
const (
	AntigravitySessionStart   = "SessionStart"
	AntigravityPreInvocation  = "PreInvocation"
	AntigravityPostToolUse    = "PostToolUse"
	AntigravityPostInvocation = "PostInvocation"
	AntigravityStop           = "Stop"
)

// AntigravityEvents lists the Antigravity hook events ShellTime registers, in the order they
// are written.
var AntigravityEvents = []string{
	AntigravitySessionStart,
	AntigravityPreInvocation,
	AntigravityPostToolUse,
	AntigravityPostInvocation,
	AntigravityStop,
}

// normalizeAntigravity maps an Antigravity hook payload (camelCase, without the event name,
// which comes from --event) onto ev.
func normalizeAntigravity(ev *Event, event string, p payload) {
	transcript := p.str("transcriptPath")
	ev.Surface = antigravitySurface(transcript)
	ev.SessionID = p.str("conversationId")
	ev.Model = p.str("modelName")
	ev.Workspace = firstWorkspace(p.strs("workspacePaths"), p.str("cwd"))

	switch strings.ToLower(event) {
	case strings.ToLower(AntigravitySessionStart):
		ev.EventType = EventSessionStart
	case strings.ToLower(AntigravityPreInvocation):
		ev.EventType = EventPromptSubmit
		// Some CLI builds leave lastUserInput out
		ev.Prompt = p.str("lastUserInput")
		p.setNumberMetadata(ev, "invocationNum", "invocation_num")
	case strings.ToLower(AntigravityPostToolUse):
		call := p.object("toolCall")
		applyToolInput(ev, call.str("name"), call["args"])
		if id := call.str("id"); id != "" {
			ev.SetMetadata("tool_call_id", id)
		}
		p.setNumberMetadata(ev, "stepIdx", "step_idx")
		if errText := p.text("error"); errText != "" {
			ev.EventType = EventToolFailure
			ev.Status = StatusError
			ev.Error = errText
		} else {
			ev.EventType = EventToolUse
			ev.Status = StatusOK
		}
	case strings.ToLower(AntigravityPostInvocation):
		ev.EventType = EventAgentResponse
		// Some builds leave content out
		ev.Response = p.str("content")
		p.setNumberMetadata(ev, "invocationNum", "invocation_num")
	case strings.ToLower(AntigravityStop):
		ev.EventType = EventStop
		ev.Status = p.str("terminationReason")
		ev.Error = p.text("error")
		p.setStringMetadata(ev, "terminationReason", "termination_reason")
		p.setNumberMetadata(ev, "executionNum", "execution_num")
		p.setBoolMetadata(ev, "fullyIdle", "fully_idle")
	default:
		ev.EventType = EventOther
	}
}

// antigravitySurface tells the CLI from the app by where the transcript is kept:
// ~/.gemini/antigravity-cli/ for agy, ~/.gemini/antigravity/ for the app.
func antigravitySurface(transcriptPath string) string {
	path := strings.ReplaceAll(transcriptPath, `\`, "/")
	switch {
	case strings.Contains(path, "/.gemini/antigravity-cli/"):
		return SurfaceCLI
	case strings.Contains(path, "/.gemini/antigravity/"):
		return SurfaceIDE
	}
	return ""
}
