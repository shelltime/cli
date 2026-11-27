package agenthook

import "strings"

const (
	passThroughEmpty    = `{}`
	passThroughContinue = `{"continue":true}`
	passThroughAllow    = `{"decision":"allow"}`
)

// PassThrough returns the reply that lets the agent carry on as if no hook ran, for a
// --agent value and a hook event:
//
//   - Cursor reads {"continue": bool} from beforeSubmitPrompt and takes {} everywhere else
//     (on stop, {} lets the agent stop).
//   - Antigravity requires a decision from Stop; the observational events take {}.
//
// An unknown or empty agent gets {}.
func PassThrough(agent, event string) []byte {
	return []byte(passThroughReply(agent, event))
}

func passThroughReply(agent, event string) string {
	canonical, _ := ParseAgent(agent)
	event = strings.TrimSpace(event)
	switch canonical {
	case AgentCursor:
		if strings.EqualFold(event, CursorBeforeSubmitPrompt) {
			return passThroughContinue
		}
	case AgentAntigravity:
		if strings.EqualFold(event, AntigravityStop) {
			return passThroughAllow
		}
	}
	return passThroughEmpty
}
