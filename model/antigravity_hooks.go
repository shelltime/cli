package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/malamtime/cli/agenthook"
)

const (
	// AntigravityHookGroup is the named hook group ShellTime owns in an Antigravity hook file.
	AntigravityHookGroup = "shelltime"
	// AntigravityHookTimeoutSeconds bounds each hook run, in whole seconds as Antigravity wants.
	AntigravityHookTimeoutSeconds = 15
	// antigravityHookAgent is the --agent value written into the commands.
	antigravityHookAgent = "agy"
)

// AntigravityHooksService registers `shelltime hook-ingest` for Antigravity's hook events in a
// project's .agents/hooks.json, which the app and the agy CLI run in that workspace (ahead of
// the global file) and the Antigravity agent reads in its Gemini API sandbox. The file's root is
// an object of named hook groups; ShellTime owns the "shelltime" group and leaves the others
// alone. The global ~/.gemini/config/hooks.json, where older versions installed the hooks, is
// only read and cleaned.
//
// Antigravity's parser is strict: a byte order mark, a comment or a trailing comma makes it
// drop the whole file. A file like that is refused, never rewritten, and the file is always
// written as strict JSON without a byte order mark.
type AntigravityHooksService struct {
	path    string
	project bool
}

// NewAntigravityHooksService manages the global ~/.gemini/config/hooks.json: Status and
// Uninstall only.
func NewAntigravityHooksService() *AntigravityHooksService {
	return &AntigravityHooksService{path: os.ExpandEnv("$HOME/.gemini/config/hooks.json")}
}

// NewAntigravityProjectHooksService manages the .agents/hooks.json of the project at root.
func NewAntigravityProjectHooksService(root string) *AntigravityHooksService {
	return &AntigravityHooksService{path: filepath.Join(root, ".agents", "hooks.json"), project: true}
}

// Path returns the hook file.
func (s *AntigravityHooksService) Path() string {
	return s.path
}

// AntigravityProjectHookCommand is the command a project's hook file runs for an Antigravity
// hook event (see projectHookCommand).
func AntigravityProjectHookCommand(event string) string {
	return projectHookCommand(antigravityHookAgent, event)
}

type antigravityHookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type antigravityHookMatcher struct {
	Matcher string                   `json:"matcher"`
	Hooks   []antigravityHookHandler `json:"hooks"`
}

// antigravityHookGroupValue builds the "shelltime" group. Tool events take matchers, the
// others a flat list of handlers. PreToolUse is never registered: a reply without a decision
// denies the tool call.
func antigravityHookGroupValue() (json.RawMessage, error) {
	var group []jsonField
	for _, event := range agenthook.AntigravityEvents {
		handler := antigravityHookHandler{
			Type:    "command",
			Command: AntigravityProjectHookCommand(event),
			Timeout: AntigravityHookTimeoutSeconds,
		}
		var value any = []antigravityHookHandler{handler}
		if event == agenthook.AntigravityPostToolUse {
			value = []antigravityHookMatcher{{Matcher: "*", Hooks: []antigravityHookHandler{handler}}}
		}
		raw, err := marshalJSONValue(value)
		if err != nil {
			return nil, err
		}
		group = append(group, jsonField{Key: event, Value: raw})
	}
	return encodeOrderedObject(group), nil
}

// Install sets the "shelltime" group of a project's file, in place when it is already there. It
// creates the file when missing.
func (s *AntigravityHooksService) Install() error {
	if !s.project {
		return ErrUserHooksRetired
	}
	data, mode, err := readJSONFileWithMode(s.path)
	if err != nil {
		return err
	}
	root, err := s.parse(data)
	if err != nil {
		return err
	}
	group, err := antigravityHookGroupValue()
	if err != nil {
		return err
	}
	root = setField(root, AntigravityHookGroup, group)
	return s.write(root, data, mode)
}

// Uninstall removes the "shelltime" group. A missing file, or one without the group, is left
// alone.
func (s *AntigravityHooksService) Uninstall() error {
	data, mode, err := readJSONFileWithMode(s.path)
	if err != nil || data == nil {
		return err
	}
	root, err := s.parse(data)
	if err != nil {
		return err
	}
	root, removed := deleteField(root, AntigravityHookGroup)
	if !removed {
		return nil
	}
	return s.write(root, data, mode)
}

// Status reports which managed events the "shelltime" group registers hook-ingest for and
// which binaries those commands run.
func (s *AntigravityHooksService) Status() (AgentHooksStatus, error) {
	status := AgentHooksStatus{}
	data, _, err := readJSONFileWithMode(s.path)
	if err != nil {
		return status, err
	}
	root, err := s.parse(data)
	if err != nil {
		return status, err
	}
	var group []jsonField
	if raw, ok := getField(root, AntigravityHookGroup); ok {
		// Uninstall removes the whole group, whatever it holds
		status.Present = true
		group, _ = decodeOrderedObject(raw)
	}
	if raw, ok := getField(group, "enabled"); ok && string(bytes.TrimSpace(raw)) == "false" {
		// A disabled group runs none of its hooks
		group = nil
	}
	for _, event := range agenthook.AntigravityEvents {
		var commands []string
		if raw, ok := getField(group, event); ok {
			var value any
			if json.Unmarshal(raw, &value) == nil {
				commands = collectHookCommands(value, commands)
			}
		}
		found := false
		for _, command := range commands {
			if strings.Contains(command, HookIngestSubcommand) {
				found = true
				status.addBinary(event, hookCommandBinary(command))
			}
		}
		if !found {
			status.MissingEvents = append(status.MissingEvents, event)
		}
	}
	status.Installed = len(status.MissingEvents) < len(agenthook.AntigravityEvents)
	return status, nil
}

// collectHookCommands appends every "command" string found in a decoded JSON value.
func collectHookCommands(value any, commands []string) []string {
	switch v := value.(type) {
	case map[string]any:
		// Antigravity runs nothing marked "enabled": false
		if enabled, ok := v["enabled"].(bool); ok && !enabled {
			return commands
		}
		if command, ok := v["command"].(string); ok {
			commands = append(commands, command)
		}
		for key, child := range v {
			if key != "command" {
				commands = collectHookCommands(child, commands)
			}
		}
	case []any:
		for _, child := range v {
			commands = collectHookCommands(child, commands)
		}
	}
	return commands
}

// parse decodes the file, refusing anything Antigravity's strict parser would drop.
func (s *AntigravityHooksService) parse(data []byte) ([]jsonField, error) {
	if bytes.HasPrefix(data, utf8BOM) {
		return nil, fmt.Errorf("%w: %s starts with a byte order mark, which makes Antigravity ignore the whole file", ErrHookFileInvalid, s.path)
	}
	root, err := decodeOrderedObject(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %s isn't strict JSON (comments and trailing commas make Antigravity ignore the whole file): %v", ErrHookFileInvalid, s.path, err)
	}
	return root, nil
}

func (s *AntigravityHooksService) write(root []jsonField, original []byte, mode os.FileMode) error {
	out, err := formatOrderedJSON(root)
	if err != nil {
		return err
	}
	if bytes.Equal(out, original) {
		return nil
	}
	return writeHookFile(s.path, out, mode)
}
