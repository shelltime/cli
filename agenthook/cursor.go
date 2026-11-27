package agenthook

import (
	"encoding/json"
	"strings"
)

// Cursor's hook events (~/.cursor/hooks.json), as registered by `shelltime cursor install`.
const (
	CursorBeforeSubmitPrompt = "beforeSubmitPrompt"
	CursorPostToolUse        = "postToolUse"
	CursorPostToolUseFailure = "postToolUseFailure"
	CursorAfterAgentResponse = "afterAgentResponse"
	CursorStop               = "stop"
	CursorSessionStart       = "sessionStart"
	CursorSessionEnd         = "sessionEnd"
)

// CursorEvents lists the Cursor hook events ShellTime registers, in the order they are
// written.
var CursorEvents = []string{
	CursorBeforeSubmitPrompt,
	CursorPostToolUse,
	CursorPostToolUseFailure,
	CursorAfterAgentResponse,
	CursorStop,
	CursorSessionStart,
	CursorSessionEnd,
}

// normalizeCursor maps a Cursor hook payload (snake_case) onto ev.
func normalizeCursor(ev *Event, event string, p payload, getenv func(string) string) {
	ev.Surface = cursorSurface(getenv)
	// Tool events sometimes come with an empty conversation_id; the daemon fills it in
	ev.SessionID = p.str("conversation_id")
	if ev.SessionID == "" {
		ev.SessionID = p.str("session_id")
	}
	ev.Model = p.str("model")
	ev.Workspace = firstWorkspace(p.strs("workspace_roots"), p.str("cwd"), getenv("CURSOR_PROJECT_DIR"))
	p.setStringMetadata(ev, "cursor_version", "cursor_version")
	p.setStringMetadata(ev, "generation_id", "generation_id")
	// A remote workspace may be a cloud agent's VM or one of the user's SSH hosts, so this is
	// kept apart from the cloud signals
	if getenv(cursorCodeRemoteEnvVar) == "true" {
		ev.SetMetadata("remote_workspace", true)
	}

	switch strings.ToLower(event) {
	case strings.ToLower(CursorBeforeSubmitPrompt):
		ev.EventType = EventPromptSubmit
		ev.Prompt = p.str("prompt")
		if paths := cursorAttachmentPaths(p); len(paths) > 0 {
			ev.SetMetadata("attachments", paths)
		}
	case strings.ToLower(CursorPostToolUse):
		ev.EventType = EventToolUse
		ev.Status = StatusOK
		applyToolInput(ev, p.str("tool_name"), p["tool_input"])
		ev.DurationMs = durationMs(p.number("duration"))
		p.setStringMetadata(ev, "tool_use_id", "tool_use_id")
		// tool_output is never collected: it can be large and hold anything the tool read
	case strings.ToLower(CursorPostToolUseFailure):
		ev.EventType = EventToolFailure
		applyToolInput(ev, p.str("tool_name"), p["tool_input"])
		ev.Status = p.str("failure_type")
		if ev.Status == "" {
			ev.Status = StatusError
		}
		ev.Error = p.text("error_message")
		ev.DurationMs = durationMs(p.number("duration"))
		p.setStringMetadata(ev, "tool_use_id", "tool_use_id")
		p.setStringMetadata(ev, "failure_type", "failure_type")
		p.setBoolMetadata(ev, "is_interrupt", "is_interrupt")
	case strings.ToLower(CursorAfterAgentResponse):
		ev.EventType = EventAgentResponse
		ev.Response = p.str("text")
	case strings.ToLower(CursorStop):
		ev.EventType = EventStop
		ev.Status = p.str("status")
		p.setNumberMetadata(ev, "loop_count", "loop_count")
	case strings.ToLower(CursorSessionStart):
		ev.EventType = EventSessionStart
		p.setBoolMetadata(ev, "is_background_agent", "is_background_agent")
		p.setStringMetadata(ev, "composer_mode", "composer_mode")
	case strings.ToLower(CursorSessionEnd):
		ev.EventType = EventSessionEnd
		ev.Status = p.str("final_status")
		ev.Error = p.text("error_message")
		p.setStringMetadata(ev, "reason", "reason")
		p.setNumberMetadata(ev, "duration_ms", "duration_ms")
	default:
		ev.EventType = EventOther
	}
}

// cursorSurface guesses whether a Cursor hook runs in the IDE or in cursor-agent. Cursor
// names neither, so this is a heuristic: the IDE is a VS Code fork and passes VS Code's
// environment to the commands it runs, which the CLI doesn't.
func cursorSurface(getenv func(string) string) string {
	for _, key := range []string{"VSCODE_PID", "VSCODE_IPC_HOOK", "VSCODE_CWD"} {
		if getenv(key) != "" {
			return SurfaceIDE
		}
	}
	return SurfaceCLI
}

func cursorAttachmentPaths(p payload) []string {
	raw, ok := p["attachments"]
	if !ok {
		return nil
	}
	var attachments []json.RawMessage
	if err := json.Unmarshal(raw, &attachments); err != nil {
		return nil
	}
	var paths []string
	for _, item := range attachments {
		var a payload
		if json.Unmarshal(item, &a) != nil {
			continue
		}
		if path := a.str("file_path"); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}
