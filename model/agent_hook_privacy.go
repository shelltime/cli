package model

import "github.com/malamtime/cli/agenthook"

// ApplyAgentHookPrivacy applies the user's privacy settings to a Cursor or Antigravity hook
// event before it leaves the machine, in this order:
//
//  1. a command matching an exclude pattern is dropped, with the tool input and response
//     that may repeat it, and the event is marked excluded;
//  2. with data masking on (the default), tokens are masked in the prompt, response,
//     command, tool input and error;
//  3. with agentHooks.sendContent off, the prompt, response and tool input are dropped.
//
// It can run more than once on the same event.
func ApplyAgentHookPrivacy(ev *agenthook.Event, cfg ShellTimeConfig) {
	if ev == nil {
		return
	}
	if ev.Command != "" && ShouldExcludeCommand(ev.Command, cfg.Exclude) {
		ev.Command = ""
		ev.ToolInput = ""
		ev.Response = ""
		ev.SetMetadata("excluded", true)
	}
	if cfg.DataMasking == nil || *cfg.DataMasking {
		for _, field := range []*string{&ev.Prompt, &ev.Response, &ev.Command, &ev.ToolInput, &ev.Error} {
			if *field != "" {
				*field = MaskSensitiveTokens(*field)
			}
		}
	}
	if !cfg.AgentHooksSendContent() {
		ev.Prompt = ""
		ev.Response = ""
		ev.ToolInput = ""
	}
}
