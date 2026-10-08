package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// claudeSyntheticModel marks assistant lines Claude Code writes itself
// (errors, "no response requested") rather than receiving from the API.
const claudeSyntheticModel = "<synthetic>"

// ClaudeProjectRoots returns the directories holding Claude Code transcripts:
// the comma-separated CLAUDE_CONFIG_DIR list if set, otherwise both
// $XDG_CONFIG_HOME/claude (default ~/.config/claude) and ~/.claude.
func ClaudeProjectRoots() []string {
	var bases []string
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); strings.TrimSpace(env) != "" {
		bases = splitPathList(env)
	} else {
		home, _ := os.UserHomeDir()
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		bases = []string{filepath.Join(xdg, "claude"), filepath.Join(home, ".claude")}
	}

	roots := make([]string, 0, len(bases))
	for _, base := range bases {
		roots = append(roots, filepath.Join(base, "projects"))
	}
	return roots
}

type claudeTranscriptLine struct {
	Type              string          `json:"type"`
	UUID              string          `json:"uuid"`
	SessionID         string          `json:"sessionId"`
	Timestamp         string          `json:"timestamp"`
	Cwd               string          `json:"cwd"`
	Version           string          `json:"version"`
	IsSidechain       bool            `json:"isSidechain"`
	IsMeta            bool            `json:"isMeta"`
	IsCompactSummary  bool            `json:"isCompactSummary"`
	IsAPIErrorMessage bool            `json:"isApiErrorMessage"`
	RequestID         string          `json:"requestId"`
	CostUSD           *float64        `json:"costUSD"`
	Origin            *claudeOrigin   `json:"origin"`
	Message           *claudeMessage  `json:"message"`
	ToolUseResult     json.RawMessage `json:"toolUseResult"`
}

type claudeOrigin struct {
	Kind string `json:"kind"`
}

type claudeMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *claudeUsage    `json:"usage"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u claudeUsage) total() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

type claudeContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

type claudeAPIRequest struct {
	naturalKey string
	sessions   []string
	ts         time.Time
	model      string
	usage      claudeUsage
	costUSD    *float64
	isError    bool
	errorText  string
}

type claudeToolUse struct {
	name   string
	params map[string]any
}

type claudeToolResult struct {
	sessions   []string
	ts         time.Time
	isError    bool
	durationMs *int
}

type claudePrompt struct {
	sessions []string
	ts       time.Time
	text     string
}

type claudeSessionMeta struct {
	start     time.Time
	cwd       string
	version   string
	versionTs time.Time
}

// claudeParser rebuilds sessions from Claude Code transcripts. Items are
// deduplicated globally: one API response is written as several lines that
// repeat the same usage, and resumed sessions copy earlier history into new
// files. A duplicated item is credited to the earliest session it appears in.
type claudeParser struct {
	apiReqs     map[backfillKey]*claudeAPIRequest
	apiOrder    []backfillKey
	toolUses    map[string]claudeToolUse
	toolResults map[string]*claudeToolResult
	resultOrder []string
	prompts     map[string]*claudePrompt
	promptOrder []string
	sessions    map[string]*claudeSessionMeta
	badLines    int
}

// ParseClaudeTranscripts reads every Claude Code transcript under roots and
// rebuilds the sessions found in them as backfill events.
func ParseClaudeTranscripts(roots []string, opts BackfillOptions) (*BackfillParseResult, error) {
	files, err := collectJSONL(roots, opts.Since)
	if err != nil {
		return nil, err
	}

	p := &claudeParser{
		apiReqs:     map[backfillKey]*claudeAPIRequest{},
		toolUses:    map[string]claudeToolUse{},
		toolResults: map[string]*claudeToolResult{},
		prompts:     map[string]*claudePrompt{},
		sessions:    map[string]*claudeSessionMeta{},
	}
	for _, file := range files {
		if err := readJSONLLines(file, p.handleLine); err != nil {
			return nil, err
		}
	}

	return &BackfillParseResult{
		Sessions: p.build(opts),
		Files:    len(files),
		BadLines: p.badLines,
	}, nil
}

func (p *claudeParser) handleLine(line []byte) {
	var l claudeTranscriptLine
	if err := json.Unmarshal(line, &l); err != nil {
		p.badLines++
		return
	}
	if l.SessionID == "" || l.Timestamp == "" {
		return
	}
	ts, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		p.badLines++
		return
	}

	meta := p.sessions[l.SessionID]
	if meta == nil {
		meta = &claudeSessionMeta{start: ts}
		p.sessions[l.SessionID] = meta
	}
	if ts.Before(meta.start) {
		meta.start = ts
	}
	if meta.cwd == "" && l.Cwd != "" {
		meta.cwd = l.Cwd
	}
	if l.Version != "" && !ts.Before(meta.versionTs) {
		meta.version = l.Version
		meta.versionTs = ts
	}

	if l.Message == nil {
		return
	}
	switch l.Type {
	case "assistant":
		p.handleAssistant(&l, ts)
	case "user":
		p.handleUser(&l, ts)
	}
}

func (p *claudeParser) handleAssistant(l *claudeTranscriptLine, ts time.Time) {
	blocks, _ := decodeClaudeContent(l.Message.Content)
	for _, b := range blocks {
		if b.Type == "tool_use" && b.ID != "" {
			if _, ok := p.toolUses[b.ID]; !ok {
				p.toolUses[b.ID] = claudeToolUse{name: b.Name, params: claudeToolParams(b.Input)}
			}
		}
	}

	usage := l.Message.Usage
	if usage == nil {
		return
	}
	if l.Message.Model == claudeSyntheticModel && !l.IsAPIErrorMessage {
		return
	}

	naturalKey := claudeRequestKey(l)
	key := newBackfillKey(AICodeClientClaudeCode, naturalKey)
	if existing, ok := p.apiReqs[key]; ok {
		existing.sessions = appendUnique(existing.sessions, l.SessionID)
		// Early lines of a streamed response can carry partial usage; keep
		// the most complete one.
		if usage.total() > existing.usage.total() {
			existing.usage = *usage
			if l.CostUSD != nil {
				existing.costUSD = l.CostUSD
			}
		}
		return
	}

	req := &claudeAPIRequest{
		naturalKey: naturalKey,
		sessions:   []string{l.SessionID},
		ts:         ts,
		model:      l.Message.Model,
		usage:      *usage,
		costUSD:    l.CostUSD,
		isError:    l.IsAPIErrorMessage,
	}
	if req.isError {
		req.errorText = CapAICodeText(claudeBlocksText(blocks), backfillMaxErrorBytes)
		if req.model == claudeSyntheticModel {
			req.model = ""
		}
	}
	p.apiReqs[key] = req
	p.apiOrder = append(p.apiOrder, key)
}

func (p *claudeParser) handleUser(l *claudeTranscriptLine, ts time.Time) {
	blocks, text := decodeClaudeContent(l.Message.Content)

	hasToolResult := false
	for _, b := range blocks {
		if b.Type != "tool_result" || b.ToolUseID == "" {
			continue
		}
		hasToolResult = true
		if existing, ok := p.toolResults[b.ToolUseID]; ok {
			existing.sessions = appendUnique(existing.sessions, l.SessionID)
			continue
		}
		p.toolResults[b.ToolUseID] = &claudeToolResult{
			sessions:   []string{l.SessionID},
			ts:         ts,
			isError:    b.IsError,
			durationMs: claudeToolDuration(l.ToolUseResult),
		}
		p.resultOrder = append(p.resultOrder, b.ToolUseID)
	}
	if hasToolResult || l.UUID == "" {
		return
	}

	// Only what a person typed counts as a prompt: not meta lines, subagent
	// prompts, compaction summaries or injected notifications.
	if l.IsMeta || l.IsSidechain || l.IsCompactSummary {
		return
	}
	if l.Origin != nil && l.Origin.Kind != "human" {
		return
	}
	if blocks != nil {
		text = claudeBlocksText(blocks)
	}
	prompt, ok := normalizeClaudePrompt(text)
	if !ok {
		return
	}

	if existing, ok := p.prompts[l.UUID]; ok {
		existing.sessions = appendUnique(existing.sessions, l.SessionID)
		return
	}
	p.prompts[l.UUID] = &claudePrompt{sessions: []string{l.SessionID}, ts: ts, text: prompt}
	p.promptOrder = append(p.promptOrder, l.UUID)
}

// earliestSession picks the session that started first among those an item
// appeared in, so copies in resumed sessions are credited to the original.
func (p *claudeParser) earliestSession(candidates []string) string {
	best := candidates[0]
	for _, c := range candidates[1:] {
		if p.sessions[c].start.Before(p.sessions[best].start) {
			best = c
		}
	}
	return best
}

func (p *claudeParser) build(opts BackfillOptions) []*BackfillSession {
	out := map[string]*BackfillSession{}
	session := func(id string) *BackfillSession {
		s := out[id]
		if s == nil {
			s = &BackfillSession{ID: id, ClientType: AICodeClientClaudeCode}
			out[id] = s
		}
		return s
	}

	for _, key := range p.apiOrder {
		r := p.apiReqs[key]
		sid := p.earliestSession(r.sessions)
		eventType := AICodeEventApiRequest
		if r.isError {
			eventType = AICodeEventApiError
		}
		e := newBackfillEvent(AICodeClientClaudeCode, eventType, "api:"+r.naturalKey, sid, r.ts)
		e.Model = r.model
		if r.isError {
			e.Error = r.errorText
		} else {
			e.InputTokens = IntRef(r.usage.InputTokens)
			e.OutputTokens = IntRef(r.usage.OutputTokens)
			e.CacheReadTokens = IntRef(r.usage.CacheReadInputTokens)
			e.CacheCreationTokens = IntRef(r.usage.CacheCreationInputTokens)
			e.CostUSD = r.costUSD
		}
		session(sid).Events = append(session(sid).Events, e)
	}

	for _, uuid := range p.promptOrder {
		pr := p.prompts[uuid]
		sid := p.earliestSession(pr.sessions)
		e := newBackfillEvent(AICodeClientClaudeCode, AICodeEventUserPrompt, "prompt:"+uuid, sid, pr.ts)
		e.PromptLength = IntRef(utf8.RuneCountInString(pr.text))
		if !opts.NoPrompts {
			e.Prompt = CapAICodeText(pr.text, backfillMaxPromptBytes)
		}
		session(sid).Events = append(session(sid).Events, e)
	}

	for _, id := range p.resultOrder {
		res := p.toolResults[id]
		use, ok := p.toolUses[id]
		if !ok {
			continue
		}
		sid := p.earliestSession(res.sessions)
		e := newBackfillEvent(AICodeClientClaudeCode, AICodeEventToolResult, "tool:"+id, sid, res.ts)
		e.ToolName = use.name
		e.Success = BoolRef(!res.isError)
		e.DurationMs = res.durationMs
		e.ToolParameters = use.params
		session(sid).Events = append(session(sid).Events, e)
	}

	machine := currentBackfillMachine()
	sessions := make([]*BackfillSession, 0, len(out))
	for id, s := range out {
		meta := p.sessions[id]
		for i := range s.Events {
			machine.apply(&s.Events[i])
			s.Events[i].Pwd = meta.cwd
			s.Events[i].AppVersion = meta.version
		}
		finalizeBackfillSession(s)
		sessions = append(sessions, s)
	}
	return sessions
}

// claudeRequestKey identifies one API response. Claude Code writes a line
// per content block, all sharing message.id and requestId.
func claudeRequestKey(l *claudeTranscriptLine) string {
	switch {
	case l.Message.ID != "" && l.RequestID != "":
		return l.Message.ID + ":" + l.RequestID
	case l.Message.ID != "":
		return l.Message.ID
	default:
		return "uuid:" + l.UUID
	}
}

// decodeClaudeContent returns the content blocks of a message, or its text
// when the content is a plain string.
func decodeClaudeContent(raw json.RawMessage) ([]claudeContentBlock, string) {
	if len(raw) == 0 {
		return nil, ""
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return nil, text
		}
		return nil, ""
	}
	var blocks []claudeContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, ""
	}
	if blocks == nil {
		blocks = []claudeContentBlock{}
	}
	return blocks, ""
}

func claudeBlocksText(blocks []claudeContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

var (
	claudeCommandNamePattern = regexp.MustCompile(`<command-name>([^<]*)</command-name>`)
	claudeCommandArgsPattern = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
)

// normalizeClaudePrompt turns the text of a user line into the prompt the
// person typed. Output of local commands and interruption markers are not
// prompts; slash commands are reduced to "/name args".
func normalizeClaudePrompt(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	for _, prefix := range []string{
		"<local-command-stdout>",
		"<local-command-stderr>",
		"<local-command-caveat>",
		"[Request interrupted",
	} {
		if strings.HasPrefix(text, prefix) {
			return "", false
		}
	}
	if m := claudeCommandNamePattern.FindStringSubmatch(text); m != nil && strings.HasPrefix(text, "<command-") {
		command := strings.TrimSpace(m[1])
		if args := claudeCommandArgsPattern.FindStringSubmatch(text); args != nil && strings.TrimSpace(args[1]) != "" {
			command += " " + strings.TrimSpace(args[1])
		}
		return command, command != ""
	}
	return text, true
}

// claudeToolParams keeps only the file path of a tool call: the server uses
// it for the files-edited list, and commands or file contents may hold
// secrets the live pipeline never uploads either.
func claudeToolParams(input json.RawMessage) map[string]any {
	if len(input) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return nil
	}
	var params map[string]any
	for _, key := range []string{"file_path", "path", "notebook_path"} {
		var value string
		if raw, ok := fields[key]; ok && json.Unmarshal(raw, &value) == nil && value != "" {
			if params == nil {
				params = map[string]any{}
			}
			params[key] = value
		}
	}
	return params
}

// claudeToolDuration reads the duration some tools record in toolUseResult.
// A timestamp difference is not used: it would include the time spent
// waiting for the user to approve the call.
func claudeToolDuration(raw json.RawMessage) *int {
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	var result struct {
		DurationMs      *float64 `json:"durationMs"`
		TotalDurationMs *float64 `json:"totalDurationMs"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil
	}
	if result.DurationMs != nil {
		return IntRef(int(*result.DurationMs))
	}
	if result.TotalDurationMs != nil {
		return IntRef(int(*result.TotalDurationMs))
	}
	return nil
}

func appendUnique(values []string, v string) []string {
	for _, existing := range values {
		if existing == v {
			return values
		}
	}
	return append(values, v)
}
