package model

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// codexFallbackModel is used for token counts logged before any turn
// context named the model, matching ccusage's choice for old rollouts.
const codexFallbackModel = "gpt-5"

// codexResponseCompleted is the event kind the live Codex OTEL pipeline
// attaches to the sse_event carrying a response's final token counts; the
// server only sums tokens of Codex events with this kind.
const codexResponseCompleted = "response.completed"

// CodexSessionRoots returns the directories holding Codex rollouts: the
// sessions and archived_sessions folders of each CODEX_HOME entry (comma
// separated), or of ~/.codex.
func CodexSessionRoots() []string {
	homes := splitPathList(os.Getenv("CODEX_HOME"))
	if len(homes) == 0 {
		home, _ := os.UserHomeDir()
		homes = []string{filepath.Join(home, ".codex")}
	}
	roots := make([]string, 0, len(homes)*2)
	for _, h := range homes {
		roots = append(roots, filepath.Join(h, "sessions"), filepath.Join(h, "archived_sessions"))
	}
	return roots
}

type codexRolloutLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexSessionMeta struct {
	ID            string `json:"id"`
	Cwd           string `json:"cwd"`
	CLIVersion    string `json:"cli_version"`
	ModelProvider string `json:"model_provider"`
}

type codexTurnContext struct {
	Cwd            string          `json:"cwd"`
	Model          string          `json:"model"`
	Effort         string          `json:"effort"`
	ApprovalPolicy string          `json:"approval_policy"`
	SandboxPolicy  json.RawMessage `json:"sandbox_policy"`
}

type codexUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

func (u codexUsage) isZero() bool {
	return u.InputTokens == 0 && u.CachedInputTokens == 0 && u.OutputTokens == 0 && u.ReasoningOutputTokens == 0
}

// minus returns u - prev with every field clamped at zero.
func (u codexUsage) minus(prev codexUsage) codexUsage {
	return codexUsage{
		InputTokens:           max(u.InputTokens-prev.InputTokens, 0),
		CachedInputTokens:     max(u.CachedInputTokens-prev.CachedInputTokens, 0),
		OutputTokens:          max(u.OutputTokens-prev.OutputTokens, 0),
		ReasoningOutputTokens: max(u.ReasoningOutputTokens-prev.ReasoningOutputTokens, 0),
		TotalTokens:           max(u.TotalTokens-prev.TotalTokens, 0),
	}
}

type codexEventMsg struct {
	Type    string          `json:"type"`
	Message string          `json:"message"`
	Kind    string          `json:"kind"`
	Info    *codexTokenInfo `json:"info"`
	Model   string          `json:"model"`
}

type codexTokenInfo struct {
	TotalTokenUsage *codexUsage `json:"total_token_usage"`
	LastTokenUsage  *codexUsage `json:"last_token_usage"`
	Model           string      `json:"model"`
}

type codexResponseItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	Action    json.RawMessage `json:"action"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type codexPendingCall struct {
	name string
	args map[string]any
	ts   time.Time
}

// codexFileState is the per-rollout state needed to turn its lines into
// events: the session, the latest turn context, the previous cumulative
// token usage and tool calls waiting for their output.
type codexFileState struct {
	path         string
	sessionID    string
	cwd          string
	cliVersion   string
	provider     string
	metaTs       time.Time
	model        string
	effort       string
	approval     string
	sandbox      string
	turnSeen     bool
	firstTurn    codexTurnContext
	prevTotal    *codexUsage
	pending      map[string]codexPendingCall
	events       []AICodeBackfillEvent
	userEvents   int
	userFallback []codexPromptCandidate
}

type codexPromptCandidate struct {
	text  string
	rawTs string
	ts    time.Time
}

type codexParser struct {
	opts     BackfillOptions
	seen     map[backfillKey]struct{}
	sessions map[string]*BackfillSession
	meta     map[string]*codexFileState
	badLines int
}

var codexRolloutUUIDPattern = regexp.MustCompile(`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.jsonl$`)

// ParseCodexRollouts reads every Codex rollout under roots and rebuilds the
// sessions found in them as backfill events.
func ParseCodexRollouts(roots []string, opts BackfillOptions) (*BackfillParseResult, error) {
	files, err := collectJSONL(roots, opts.Since)
	if err != nil {
		return nil, err
	}
	// Forked rollouts copy their parent's history. Reading files in the
	// order they started credits copied items to the original session.
	starts := make(map[string]string, len(files))
	for _, f := range files {
		starts[f] = codexFirstTimestamp(f)
	}
	sort.SliceStable(files, func(i, j int) bool { return starts[files[i]] < starts[files[j]] })

	p := &codexParser{
		opts:     opts,
		seen:     map[backfillKey]struct{}{},
		sessions: map[string]*BackfillSession{},
		meta:     map[string]*codexFileState{},
	}
	for _, f := range files {
		if err := p.parseFile(f); err != nil {
			return nil, err
		}
	}

	machine := currentBackfillMachine()
	sessions := make([]*BackfillSession, 0, len(p.sessions))
	for id, s := range p.sessions {
		meta := p.meta[id]
		for i := range s.Events {
			machine.apply(&s.Events[i])
			s.Events[i].ConversationID = id
			s.Events[i].Pwd = meta.cwd
			s.Events[i].AppVersion = meta.cliVersion
		}
		finalizeBackfillSession(s)
		if len(s.Events) > 0 {
			sessions = append(sessions, s)
		}
	}
	return &BackfillParseResult{Sessions: sessions, Files: len(files), BadLines: p.badLines}, nil
}

// codexFirstTimestamp returns the timestamp of a rollout's first line, or
// "" when it can't be read.
func codexFirstTimestamp(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadBytes('\n')
	var l codexRolloutLine
	if json.Unmarshal(line, &l) != nil {
		return ""
	}
	return l.Timestamp
}

func (p *codexParser) parseFile(path string) error {
	st := &codexFileState{path: path, pending: map[string]codexPendingCall{}}
	if m := codexRolloutUUIDPattern.FindStringSubmatch(filepath.Base(path)); m != nil {
		st.sessionID = m[1]
	}

	err := readJSONLLines(path, func(line []byte) {
		var l codexRolloutLine
		if err := json.Unmarshal(line, &l); err != nil {
			p.badLines++
			return
		}
		if l.Timestamp == "" || len(l.Payload) == 0 {
			return
		}
		ts, err := time.Parse(time.RFC3339Nano, l.Timestamp)
		if err != nil {
			p.badLines++
			return
		}
		switch l.Type {
		case "session_meta":
			p.handleSessionMeta(st, l.Payload, ts)
		case "turn_context":
			p.handleTurnContext(st, l.Payload)
		case "event_msg":
			p.handleEventMsg(st, l.Payload, l.Timestamp, ts)
		case "response_item":
			p.handleResponseItem(st, l.Payload, l.Timestamp, ts)
		}
	})
	if err != nil {
		return err
	}
	if st.sessionID == "" {
		return nil
	}

	// Old rollouts don't log user_message events; fall back to the user
	// messages of the conversation itself.
	if st.userEvents == 0 {
		for _, c := range st.userFallback {
			p.addPrompt(st, c.text, c.rawTs, c.ts)
		}
	}
	if !st.metaTs.IsZero() {
		p.addConversationStart(st)
	}

	if _, ok := p.meta[st.sessionID]; !ok {
		p.meta[st.sessionID] = st
	}
	if len(st.events) == 0 {
		return nil
	}
	s := p.sessions[st.sessionID]
	if s == nil {
		s = &BackfillSession{ID: st.sessionID, ClientType: AICodeClientCodex}
		p.sessions[st.sessionID] = s
	}
	s.Events = append(s.Events, st.events...)
	return nil
}

// keep reports whether an item is new, recording it as seen. Copies of the
// same item in forked rollouts are dropped.
func (p *codexParser) keep(naturalKey string) bool {
	k := newBackfillKey(AICodeClientCodex, naturalKey)
	if _, ok := p.seen[k]; ok {
		return false
	}
	p.seen[k] = struct{}{}
	return true
}

func (p *codexParser) handleSessionMeta(st *codexFileState, raw json.RawMessage, ts time.Time) {
	if !st.metaTs.IsZero() {
		// Forked rollouts can repeat their parent's metadata; the first
		// entry describes this file.
		return
	}
	var meta codexSessionMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		p.badLines++
		return
	}
	if meta.ID != "" {
		st.sessionID = meta.ID
	}
	st.metaTs = ts
	st.cwd = meta.Cwd
	st.cliVersion = meta.CLIVersion
	st.provider = meta.ModelProvider
}

func (p *codexParser) handleTurnContext(st *codexFileState, raw json.RawMessage) {
	var tc codexTurnContext
	if err := json.Unmarshal(raw, &tc); err != nil {
		p.badLines++
		return
	}
	if tc.Model != "" {
		st.model = tc.Model
	}
	st.effort = tc.Effort
	st.approval = tc.ApprovalPolicy
	st.sandbox = codexSandboxMode(tc.SandboxPolicy)
	if st.cwd == "" {
		st.cwd = tc.Cwd
	}
	if !st.turnSeen {
		st.turnSeen = true
		st.firstTurn = tc
	}
}

func (p *codexParser) handleEventMsg(st *codexFileState, raw json.RawMessage, rawTs string, ts time.Time) {
	var msg codexEventMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		p.badLines++
		return
	}
	switch msg.Type {
	case "user_message":
		st.userEvents++
		if msg.Kind != "" && msg.Kind != "plain" {
			return
		}
		p.addPrompt(st, msg.Message, rawTs, ts)
	case "token_count":
		p.handleTokenCount(st, &msg, rawTs, ts)
	}
}

func (p *codexParser) addPrompt(st *codexFileState, text, rawTs string, ts time.Time) {
	text = strings.TrimSpace(text)
	if text == "" || codexIsContextMessage(text) {
		return
	}
	naturalKey := "prompt:" + rawTs + "|" + text
	if !p.keep(naturalKey) {
		return
	}
	e := newBackfillEvent(AICodeClientCodex, AICodeEventUserPrompt, naturalKey, st.sessionID, ts)
	e.PromptLength = IntRef(utf8.RuneCountInString(text))
	if !p.opts.NoPrompts {
		e.Prompt = CapAICodeText(text, backfillMaxPromptBytes)
	}
	st.events = append(st.events, e)
}

func (p *codexParser) handleTokenCount(st *codexFileState, msg *codexEventMsg, rawTs string, ts time.Time) {
	info := msg.Info
	if info == nil {
		return
	}
	total := info.TotalTokenUsage
	if total != nil && st.prevTotal != nil && *total == *st.prevTotal {
		// Codex re-emits the last count (e.g. alongside rate limit updates).
		return
	}

	var usage codexUsage
	switch {
	case info.LastTokenUsage != nil:
		usage = *info.LastTokenUsage
	case total != nil && st.prevTotal != nil:
		usage = total.minus(*st.prevTotal)
	case total != nil:
		usage = *total
	}
	if total != nil {
		prev := *total
		st.prevTotal = &prev
	}
	if usage.isZero() {
		return
	}

	model := st.model
	if model == "" {
		model = info.Model
	}
	if model == "" {
		model = msg.Model
	}
	if model == "" {
		model = codexFallbackModel
	}

	// The model is left out of the key: a forked rollout may copy a count
	// before the turn context that names its model.
	naturalKey := fmt.Sprintf("tokens:%s|%d|%d|%d|%d|%d", rawTs,
		usage.InputTokens, usage.CachedInputTokens, usage.OutputTokens, usage.ReasoningOutputTokens, usage.TotalTokens)
	if !p.keep(naturalKey) {
		return
	}

	e := newBackfillEvent(AICodeClientCodex, AICodeEventSSEEvent, naturalKey, st.sessionID, ts)
	e.EventKind = codexResponseCompleted
	e.Model = model
	// Same semantics as the live pipeline: input includes cached tokens.
	e.InputTokens = IntRef(usage.InputTokens)
	e.CacheReadTokens = IntRef(usage.CachedInputTokens)
	e.OutputTokens = IntRef(usage.OutputTokens)
	e.ReasoningTokens = IntRef(usage.ReasoningOutputTokens)
	st.events = append(st.events, e)
}

func (p *codexParser) handleResponseItem(st *codexFileState, raw json.RawMessage, rawTs string, ts time.Time) {
	var item codexResponseItem
	if err := json.Unmarshal(raw, &item); err != nil {
		p.badLines++
		return
	}
	switch item.Type {
	case "message":
		if item.Role != "user" {
			return
		}
		var parts []string
		for _, c := range item.Content {
			if c.Type == "input_text" && strings.TrimSpace(c.Text) != "" {
				parts = append(parts, c.Text)
			}
		}
		st.userFallback = append(st.userFallback, codexPromptCandidate{text: strings.Join(parts, "\n"), rawTs: rawTs, ts: ts})
	case "function_call":
		st.pending[item.CallID] = codexPendingCall{name: item.Name, args: codexToolArguments(item.Arguments), ts: ts}
	case "custom_tool_call":
		st.pending[item.CallID] = codexPendingCall{name: item.Name, args: codexCapArgs(map[string]any{"input": item.Input}), ts: ts}
	case "local_shell_call":
		var action map[string]any
		_ = json.Unmarshal(item.Action, &action)
		st.pending[item.CallID] = codexPendingCall{name: "local_shell", args: codexCapArgs(action), ts: ts}
	case "function_call_output", "custom_tool_call_output":
		call, ok := st.pending[item.CallID]
		if !ok || item.CallID == "" {
			return
		}
		delete(st.pending, item.CallID)
		naturalKey := "tool:" + item.CallID
		if !p.keep(naturalKey) {
			return
		}
		e := newBackfillEvent(AICodeClientCodex, AICodeEventToolResult, naturalKey, st.sessionID, ts)
		e.ToolName = call.name
		e.CallID = item.CallID
		e.ToolArguments = call.args
		e.Success, e.DurationMs = codexToolOutcome(item.Output)
		st.events = append(st.events, e)
	}
}

func (p *codexParser) addConversationStart(st *codexFileState) {
	naturalKey := "conv:" + st.sessionID
	if !p.keep(naturalKey) {
		return
	}
	e := newBackfillEvent(AICodeClientCodex, AICodeEventConversationStarts, naturalKey, st.sessionID, st.metaTs)
	e.Provider = st.provider
	if st.turnSeen {
		e.Model = st.firstTurn.Model
		e.ReasoningEffort = st.firstTurn.Effort
		e.ApprovalPolicy = st.firstTurn.ApprovalPolicy
		e.SandboxPolicy = codexSandboxMode(st.firstTurn.SandboxPolicy)
	}
	st.events = append(st.events, e)
}

// codexIsContextMessage reports whether a user message is context Codex
// injects (instructions, environment) rather than something typed.
func codexIsContextMessage(text string) bool {
	for _, prefix := range []string{"<environment_context>", "<user_instructions>", "<user_shell_command>", "# AGENTS.md instructions"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// codexSandboxMode reads the sandbox policy, which is a plain string in some
// Codex versions and an object with a mode in others.
func codexSandboxMode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Mode string `json:"mode"`
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if obj.Mode != "" {
			return obj.Mode
		}
		return obj.Type
	}
	return ""
}

func codexToolArguments(arguments string) map[string]any {
	if arguments == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return nil
	}
	return codexCapArgs(args)
}

// codexCapArgs drops tool arguments too large to upload, such as whole
// patches, keeping batches small.
func codexCapArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	buf, err := json.Marshal(args)
	if err != nil || len(buf) > backfillMaxToolArgsBytes {
		return nil
	}
	return args
}

var (
	codexExitCodePattern = regexp.MustCompile(`(?m)^Exit code: (-?\d+)`)
	codexWallTimePattern = regexp.MustCompile(`(?m)^Wall time: ([0-9.]+) seconds`)
)

// codexToolOutcome reads success and duration from a tool output. Shell
// tools report them either as JSON metadata or as "Exit code:" / "Wall
// time:" lines; other tools report neither, and both stay nil.
func codexToolOutcome(raw json.RawMessage) (*bool, *int) {
	if len(raw) == 0 {
		return nil, nil
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		// Some versions store the output as an object.
		text = string(raw)
	}

	var structured struct {
		Metadata *struct {
			ExitCode        *int     `json:"exit_code"`
			DurationSeconds *float64 `json:"duration_seconds"`
		} `json:"metadata"`
	}
	if json.Unmarshal([]byte(text), &structured) == nil && structured.Metadata != nil {
		var success *bool
		var duration *int
		if structured.Metadata.ExitCode != nil {
			success = BoolRef(*structured.Metadata.ExitCode == 0)
		}
		if structured.Metadata.DurationSeconds != nil {
			duration = IntRef(int(*structured.Metadata.DurationSeconds * 1000))
		}
		return success, duration
	}

	var success *bool
	var duration *int
	if m := codexExitCodePattern.FindStringSubmatch(text); m != nil {
		if code, err := strconv.Atoi(m[1]); err == nil {
			success = BoolRef(code == 0)
		}
	}
	if m := codexWallTimePattern.FindStringSubmatch(text); m != nil {
		if secs, err := strconv.ParseFloat(m[1], 64); err == nil {
			duration = IntRef(int(secs * 1000))
		}
	}
	return success, duration
}
