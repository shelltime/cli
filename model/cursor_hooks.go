package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/malamtime/cli/agenthook"
)

// ErrHookFileInvalid is returned for an agent hook file ShellTime won't rewrite: one that isn't
// strict JSON (a byte order mark, comments, trailing commas, a syntax error) or not shaped as
// the agent expects. The file is left as it is.
var ErrHookFileInvalid = errors.New("hook file isn't valid JSON")

var utf8BOM = []byte("\xEF\xBB\xBF")

// cursorHookMarker identifies the commands ShellTime registers in ~/.cursor/hooks.json.
const cursorHookMarker = HookIngestSubcommand + " --agent cursor"

// CursorHooksService registers `shelltime hook-ingest` for Cursor's hook events in the user's
// ~/.cursor/hooks.json, which both the IDE and cursor-agent read, or in a project's
// .cursor/hooks.json, the only one Cursor Cloud Agents run:
//
//	{"version": 1, "hooks": {"<event>": [{"command": "…"}]}}
//
// Every other hook and key stays as the user wrote it, in its order.
type CursorHooksService struct {
	path string
	// project writes commands that run `--project` (see ProjectHookFlag)
	project bool
}

func NewCursorHooksService() *CursorHooksService {
	return &CursorHooksService{path: os.ExpandEnv("$HOME/.cursor/hooks.json")}
}

// NewCursorProjectHooksService manages the .cursor/hooks.json of the project at root. Cursor
// runs it in that project next to the user's own hooks, and Cloud Agents run only it, never
// ~/.cursor/hooks.json.
func NewCursorProjectHooksService(root string) *CursorHooksService {
	return &CursorHooksService{path: filepath.Join(root, ".cursor", "hooks.json"), project: true}
}

// Path returns the hook file.
func (s *CursorHooksService) Path() string {
	return s.path
}

// CursorHookCommand is the command registered for a Cursor hook event.
func CursorHookCommand(bin, event string) string {
	return hookCommand(bin, HookIngestSubcommand, "--agent", agenthook.AgentCursor, "--event", event)
}

func (s *CursorHooksService) command(bin, event string) string {
	if s.project {
		return CursorHookCommand(bin, event) + " " + ProjectHookFlag
	}
	return CursorHookCommand(bin, event)
}

// Install registers bin for every managed event. It creates the file when missing, sets
// "version": 1 only when absent, and updates a ShellTime command already there in place (a
// new binary path), so running it again changes nothing else.
func (s *CursorHooksService) Install(bin string) error {
	data, mode, err := readJSONFileWithMode(s.path)
	if err != nil {
		return err
	}
	root, hooks, err := s.parse(data)
	if err != nil {
		return err
	}

	if _, ok := getField(root, "version"); !ok {
		root = append([]jsonField{{Key: "version", Value: json.RawMessage("1")}}, root...)
	}

	managed := map[string]bool{}
	for _, event := range agenthook.CursorEvents {
		managed[event] = true
	}
	// ShellTime commands under events it no longer registers go
	var kept []jsonField
	for _, f := range hooks {
		if managed[f.Key] {
			kept = append(kept, f)
			continue
		}
		entries, err := decodeJSONArray(f.Value)
		if err != nil {
			kept = append(kept, f)
			continue
		}
		rest, removed := removeHookEntries(entries, cursorHookMarker)
		switch {
		case removed == 0:
			kept = append(kept, f)
		case len(rest) > 0:
			kept = append(kept, jsonField{Key: f.Key, Value: encodeJSONArray(rest)})
		}
	}
	hooks = kept

	for _, event := range agenthook.CursorEvents {
		var entries []json.RawMessage
		if raw, ok := getField(hooks, event); ok {
			if entries, err = decodeJSONArray(raw); err != nil {
				return fmt.Errorf("%w: hooks.%s in %s isn't an array", ErrHookFileInvalid, event, s.path)
			}
		}
		entries, err = upsertCursorHookEntry(entries, s.command(bin, event))
		if err != nil {
			return err
		}
		hooks = setField(hooks, event, encodeJSONArray(entries))
	}
	root = setField(root, "hooks", encodeOrderedObject(hooks))
	return s.write(root, data, mode)
}

// Uninstall removes the ShellTime commands, and an event's array only when that left it empty.
// A missing file, or one without ShellTime commands, is left alone.
func (s *CursorHooksService) Uninstall() error {
	data, mode, err := readJSONFileWithMode(s.path)
	if err != nil || data == nil {
		return err
	}
	root, hooks, err := s.parse(data)
	if err != nil {
		return err
	}
	if _, ok := getField(root, "hooks"); !ok {
		return nil
	}

	removedAny := false
	kept := []jsonField{}
	for _, f := range hooks {
		entries, err := decodeJSONArray(f.Value)
		if err != nil {
			kept = append(kept, f)
			continue
		}
		rest, removed := removeHookEntries(entries, cursorHookMarker)
		if removed == 0 {
			kept = append(kept, f)
			continue
		}
		removedAny = true
		if len(rest) > 0 {
			kept = append(kept, jsonField{Key: f.Key, Value: encodeJSONArray(rest)})
		}
	}
	if !removedAny {
		return nil
	}
	root = setField(root, "hooks", encodeOrderedObject(kept))
	return s.write(root, data, mode)
}

// Status reports which managed events have a ShellTime command and which binaries they run.
func (s *CursorHooksService) Status() (AgentHooksStatus, error) {
	status := AgentHooksStatus{}
	data, _, err := readJSONFileWithMode(s.path)
	if err != nil {
		return status, err
	}
	_, hooks, err := s.parse(data)
	if err != nil {
		return status, err
	}
	for _, event := range agenthook.CursorEvents {
		found := false
		if raw, ok := getField(hooks, event); ok {
			entries, _ := decodeJSONArray(raw)
			for _, entry := range entries {
				if _, command, ok := commandField(entry); ok && strings.Contains(command, cursorHookMarker) {
					found = true
					status.addBinary(hookCommandBinary(command))
				}
			}
		}
		if !found {
			status.MissingEvents = append(status.MissingEvents, event)
		}
	}
	status.Installed = len(status.MissingEvents) < len(agenthook.CursorEvents)
	return status, nil
}

// parse decodes the file into its root fields and its hooks object.
func (s *CursorHooksService) parse(data []byte) (root, hooks []jsonField, err error) {
	if bytes.HasPrefix(data, utf8BOM) {
		return nil, nil, fmt.Errorf("%w: %s starts with a byte order mark", ErrHookFileInvalid, s.path)
	}
	root, err = decodeOrderedObject(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %v", ErrHookFileInvalid, s.path, err)
	}
	if raw, ok := getField(root, "hooks"); ok {
		if hooks, err = decodeOrderedObject(raw); err != nil {
			return nil, nil, fmt.Errorf("%w: hooks in %s isn't an object", ErrHookFileInvalid, s.path)
		}
	}
	return root, hooks, nil
}

func (s *CursorHooksService) write(root []jsonField, original []byte, mode os.FileMode) error {
	out, err := formatOrderedJSON(root)
	if err != nil {
		return err
	}
	if bytes.Equal(out, original) {
		return nil
	}
	return writeHookFile(s.path, out, mode)
}

// upsertCursorHookEntry sets command on the first ShellTime entry, keeping its place and any
// other keys, drops further ShellTime entries, and appends one when there is none.
func upsertCursorHookEntry(entries []json.RawMessage, command string) ([]json.RawMessage, error) {
	value, err := marshalJSONValue(command)
	if err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, 0, len(entries)+1)
	placed := false
	for _, entry := range entries {
		fields, existing, ok := commandField(entry)
		if !ok || !strings.Contains(existing, cursorHookMarker) {
			out = append(out, entry)
			continue
		}
		if placed {
			continue
		}
		out = append(out, encodeOrderedObject(setField(fields, "command", value)))
		placed = true
	}
	if !placed {
		out = append(out, encodeOrderedObject([]jsonField{{Key: "command", Value: value}}))
	}
	return out, nil
}

// removeHookEntries drops the entries whose command contains marker and returns the rest and
// how many it dropped.
func removeHookEntries(entries []json.RawMessage, marker string) ([]json.RawMessage, int) {
	var rest []json.RawMessage
	removed := 0
	for _, entry := range entries {
		if _, command, ok := commandField(entry); ok && strings.Contains(command, marker) {
			removed++
			continue
		}
		rest = append(rest, entry)
	}
	return rest, removed
}
