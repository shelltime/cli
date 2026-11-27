package agenthook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrUnknownAgent is returned for a --agent value ParseAgent doesn't know.
	ErrUnknownAgent = errors.New("unknown agent")
	// ErrEmptyPayload is returned when the hook got nothing on stdin.
	ErrEmptyPayload = errors.New("empty hook payload")
	// ErrNoEventName is returned when neither --event nor the payload names the event.
	ErrNoEventName = errors.New("no hook event name")
)

var utf8BOM = []byte("\xEF\xBB\xBF")

// Normalize turns one hook payload into an Event. agent is a --agent value (cursor, agy or
// antigravity) and event the --event value; when that is empty, the event name the payload
// carries is used. getenv reads the hook's environment (nil reads nothing). A payload that
// isn't a JSON object gives no event.
func Normalize(agent, event string, stdin []byte, now time.Time, getenv func(string) string) (Event, error) {
	canonical, ok := ParseAgent(agent)
	if !ok {
		return Event{}, fmt.Errorf("%w %q", ErrUnknownAgent, agent)
	}
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	p, err := parsePayload(stdin)
	if err != nil {
		return Event{}, err
	}
	return normalizePayload(canonical, event, p, now, getenv)
}

// NormalizeTruncated turns the start of a hook payload too large to read whole into an Event.
// The top-level fields that end before the cut are read as Normalize reads them (agents put
// the conversation, model and workspace first, and the bulk, like a tool's output, last); the
// rest is lost, and the event's metadata says truncated. A prefix that holds nothing usable
// still gives an event of the agent and the --event name, so the hook is counted.
func NormalizeTruncated(agent, event string, prefix []byte, now time.Time, getenv func(string) string) (Event, error) {
	canonical, ok := ParseAgent(agent)
	if !ok {
		return Event{}, fmt.Errorf("%w %q", ErrUnknownAgent, agent)
	}
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	ev, err := normalizePayload(canonical, event, parsePayloadPrefix(prefix), now, getenv)
	if err != nil {
		return Event{}, err
	}
	ev.SetMetadata("truncated", true)
	return ev, nil
}

func normalizePayload(canonical, event string, p payload, now time.Time, getenv func(string) string) (Event, error) {
	event = strings.TrimSpace(event)
	if event == "" {
		event = p.eventName()
	}
	if event == "" {
		return Event{}, ErrNoEventName
	}

	ev := Event{
		ID:            NewID(),
		Agent:         canonical,
		HookEventName: event,
		Timestamp:     now.UTC().Format(time.RFC3339Nano),
	}
	switch canonical {
	case AgentCursor:
		normalizeCursor(&ev, event, p, getenv)
	case AgentAntigravity:
		normalizeAntigravity(&ev, event, p)
	}
	ev.Cloud = cloudSignal(canonical, p, getenv)
	capEvent(&ev)
	return ev, nil
}

// ResolveEventName returns event, or when it is empty the event name the payload carries
// (Cursor's hook_event_name, newer Antigravity builds' hookEventName), also in a payload cut
// short before its end. Malformed payloads give "".
func ResolveEventName(event string, stdin []byte) string {
	if event = strings.TrimSpace(event); event != "" {
		return event
	}
	p, err := parsePayload(stdin)
	if err != nil {
		p = parsePayloadPrefix(stdin)
	}
	return p.eventName()
}

// payload is a hook's stdin object, read field by field: a field of an unexpected type is
// passed over instead of failing the whole event.
type payload map[string]json.RawMessage

func parsePayload(stdin []byte) (payload, error) {
	data := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(stdin), utf8BOM))
	if len(data) == 0 {
		return nil, ErrEmptyPayload
	}
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid hook payload: %w", err)
	}
	if p == nil {
		return nil, fmt.Errorf("invalid hook payload: not a JSON object")
	}
	return p, nil
}

// parsePayloadPrefix reads the top-level fields of a JSON object cut short: every field whose
// value ends before the cut, in order, stopping at the first one that doesn't. Anything that
// doesn't start as an object gives an empty payload.
func parsePayloadPrefix(prefix []byte) payload {
	data := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(prefix), utf8BOM))
	p := payload{}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return p
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		key, ok := tok.(string)
		if !ok {
			break
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			break
		}
		p[key] = value
	}
	return p
}

func (p payload) eventName() string {
	if name := p.str("hook_event_name"); name != "" {
		return name
	}
	return p.str("hookEventName")
}

// str returns a string field. A number or a boolean is returned as written; anything else
// gives "".
func (p payload) str(key string) string {
	return scalarString(p[key])
}

// text returns a field as text: a string as is, an object or array as compact JSON, false
// and null as "".
func (p payload) text(key string) string {
	return flexText(p[key])
}

func (p payload) strs(key string) []string {
	raw, ok := p[key]
	if !ok {
		return nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		if s := scalarString(raw); s != "" {
			return []string{s}
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := scalarString(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (p payload) number(key string) (float64, bool) {
	raw, ok := p[key]
	if !ok {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// nonNegInt reads a non-negative integer field (JSON integer or ASCII decimal string).
// Booleans, floats with fractional parts, negative numbers and values that don't fit in int
// return false so bad counters never silently become zero.
func (p payload) nonNegInt(key string) (int, bool) {
	raw, ok := p[key]
	if !ok {
		return 0, false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0, false
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, false
		}
		if s == "" {
			return 0, false
		}
		for i := 0; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return 0, false
			}
		}
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 {
			return 0, false
		}
		return v, true
	}
	// Reject booleans, objects, arrays, null and decimals
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, false
		}
	}
	v, err := strconv.Atoi(string(raw))
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

func intPtr(v int) *int {
	return &v
}

func (p payload) boolean(key string) (bool, bool) {
	raw, ok := p[key]
	if !ok {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b, true
	}
	return false, false
}

func (p payload) object(key string) payload {
	raw, ok := p[key]
	if !ok {
		return nil
	}
	var obj payload
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

// setNumberMetadata copies a numeric field into metadata, as an integer when it is one.
func (p payload) setNumberMetadata(ev *Event, key, metaKey string) {
	if f, ok := p.number(key); ok {
		if f == math.Trunc(f) && math.Abs(f) < 1<<53 {
			ev.SetMetadata(metaKey, int64(f))
		} else {
			ev.SetMetadata(metaKey, f)
		}
	}
}

func (p payload) setBoolMetadata(ev *Event, key, metaKey string) {
	if b, ok := p.boolean(key); ok {
		ev.SetMetadata(metaKey, b)
	}
}

func (p payload) setStringMetadata(ev *Event, key, metaKey string) {
	if s := p.str(key); s != "" {
		ev.SetMetadata(metaKey, s)
	}
}

func scalarString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
		return ""
	case '{', '[', 'n':
		return ""
	default:
		// a number, true or false
		return string(raw)
	}
}

func flexText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
		return ""
	case 'n', 'f':
		// null, false: no error
		return ""
	case '{', '[':
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err == nil {
			return buf.String()
		}
		return string(raw)
	default:
		return string(raw)
	}
}

// durationMs reads a duration in milliseconds, rounded; negative values give 0.
func durationMs(f float64, ok bool) int64 {
	if !ok || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return int64(math.Round(f))
}

// workspacePath turns a workspace entry into a path: file:// URIs are decoded.
func workspacePath(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "file://") {
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			return u.Path
		}
	}
	return s
}

// firstWorkspace returns the first non-empty workspace entry, else fallback.
func firstWorkspace(roots []string, fallbacks ...string) string {
	for _, r := range roots {
		if w := workspacePath(r); w != "" {
			return w
		}
	}
	for _, f := range fallbacks {
		if w := workspacePath(f); w != "" {
			return w
		}
	}
	return ""
}

// toolInputKeys name the keys a tool's input holds its shell command, file and MCP server
// under, across Cursor's and Antigravity's tools.
var (
	commandInputKeys   = []string{"command", "CommandLine"}
	filePathInputKeys  = []string{"file_path", "path", "target_file", "AbsolutePath", "TargetFile", "TargetPath"}
	mcpServerInputKeys = []string{"server", "server_name"}
)

// applyToolInput fills the tool fields of ev from a tool name and its input, which may be a
// JSON object or a string holding one (Cursor sends either).
func applyToolInput(ev *Event, toolName string, input json.RawMessage) {
	ev.ToolName = toolName
	ev.MCPServer = mcpServerFromToolName(toolName)

	input = bytes.TrimSpace(input)
	if len(input) == 0 || bytes.Equal(input, []byte("null")) {
		return
	}
	if input[0] == '"' {
		var s string
		if err := json.Unmarshal(input, &s); err != nil {
			return
		}
		trimmed := strings.TrimSpace(s)
		if !strings.HasPrefix(trimmed, "{") || !json.Valid([]byte(trimmed)) {
			// Not JSON: keep the text, nothing to read keys from
			ev.ToolInput = s
			return
		}
		input = json.RawMessage(trimmed)
	}

	var buf bytes.Buffer
	if err := json.Compact(&buf, input); err != nil {
		return
	}
	ev.ToolInput = buf.String()

	var obj payload
	if err := json.Unmarshal(input, &obj); err != nil || obj == nil {
		return
	}
	ev.Command = firstString(obj, commandInputKeys)
	ev.FilePath = firstString(obj, filePathInputKeys)
	if ev.MCPServer == "" {
		ev.MCPServer = firstString(obj, mcpServerInputKeys)
	}
}

func firstString(obj payload, keys []string) string {
	for _, k := range keys {
		raw, ok := obj[k]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// mcpServerFromToolName reads the MCP server from a tool name like mcp__<server>__<tool> or
// MCP:<server>[:<tool>].
func mcpServerFromToolName(name string) string {
	name = strings.TrimSpace(name)
	if rest, ok := strings.CutPrefix(name, "mcp__"); ok {
		server, _, _ := strings.Cut(rest, "__")
		return strings.TrimSpace(server)
	}
	if len(name) > 4 && strings.EqualFold(name[:4], "mcp:") {
		rest := strings.TrimSpace(name[4:])
		server, _, _ := strings.Cut(rest, ":")
		return strings.TrimSpace(server)
	}
	return ""
}

// capEvent cuts every text field to MaxTextBytes, at a UTF-8 boundary.
func capEvent(ev *Event) {
	for _, f := range []*string{
		&ev.SessionID, &ev.HookEventName, &ev.Workspace, &ev.Model, &ev.ToolName,
		&ev.MCPServer, &ev.Command, &ev.FilePath, &ev.Status, &ev.Error,
		&ev.Prompt, &ev.Response, &ev.ToolInput,
	} {
		*f = capText(*f, MaxTextBytes)
	}
	for k, v := range ev.Metadata {
		if s, ok := v.(string); ok {
			ev.Metadata[k] = capText(s, MaxTextBytes)
		}
	}
}

func capText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
