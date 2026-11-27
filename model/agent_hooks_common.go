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

// HookIngestSubcommand is what every agent hook command runs: `… shelltime hook-ingest --agent …`.
const HookIngestSubcommand = "hook-ingest"

// The binaries a project hook command looks for before it records anything.
const (
	ProjectHookCLI    = "shelltime"
	ProjectHookDaemon = "shelltime-daemon"
)

const (
	// ProjectHookFlag marks the hook-ingest runs of a project's hook file. An older version may
	// have left its hooks in the user's own hook file, which the agent runs next to the
	// project's, so such a run leaves the event to those when they record it too.
	ProjectHookFlag = "--project"
	// projectHookPath is added to the PATH a project hook command searches: where the install
	// script puts shelltime and shelltime-daemon when it doesn't use Homebrew, then Homebrew's
	// folders, which an app started from the Dock may not have on its PATH.
	projectHookPath = "$HOME/.shelltime/bin:/opt/homebrew/bin:/usr/local/bin"
)

// ProjectHookSearchDirs are the folders a project hook command searches after PATH, for home.
func ProjectHookSearchDirs(home string) []string {
	return []string{filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin"), "/opt/homebrew/bin", "/usr/local/bin"}
}

// ErrProjectHooksUnsupported is returned for installing project hooks on Windows: their command
// is POSIX sh, and shelltime-daemon, which they record through, runs on macOS and Linux only.
var ErrProjectHooksUnsupported = errors.New("the Cursor and Antigravity hooks need shelltime-daemon, which runs on macOS and Linux only")

// ErrHomeIsNotAProject is returned for a project root that is the user's home folder, whose
// .cursor/hooks.json is Cursor's user-level hook file, not a project's.
var ErrHomeIsNotAProject = errors.New("the home folder isn't a project; run this inside the project's repository")

// projectHookCommand is the command a project's hook file runs for an agent's event (agent is
// the --agent value). The file is committed and runs on every machine that checks the
// repository out, a cloud agent's VM included, and the agents disagree on the folder a hook
// runs in, so the command is self-contained POSIX sh: where the shelltime CLI and its daemon are
// installed (on PATH, or in ProjectHookSearchDirs) it hands the event to hook-ingest; anywhere
// else it answers with the reply that lets the agent carry on, so the hook never fails. A
// shelltime from before hook-ingest exits non-zero with its error on stderr; its stdout is
// dropped and the reply answers instead.
func projectHookCommand(agent, event string) string {
	return fmt.Sprintf("PATH=$PATH:%s; command -v %s >/dev/null 2>&1 && command -v %s >/dev/null 2>&1 && out=$(%s %s --agent %s --event %s %s) && printf '%%s\\n' \"$out\" || echo '%s'",
		projectHookPath, ProjectHookCLI, ProjectHookDaemon,
		ProjectHookCLI, HookIngestSubcommand, agent, event, ProjectHookFlag,
		agenthook.PassThrough(agent, event))
}

// ProjectHooksRoot returns the root of the project dir is in, where its agents look for project
// hook files: the nearest folder up from dir that holds .git (a folder, or a file in a worktree
// or submodule), else dir itself. The user's home folder is no project (ErrHomeIsNotAProject),
// and the walk stops below it: a dotfiles repository in ~/.git makes no folder in home part of
// one.
func ProjectHooksRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	home, homeErr := os.UserHomeDir()
	isHome := func(d string) bool { return homeErr == nil && sameFile(d, home) }
	if isHome(dir) {
		return "", ErrHomeIsNotAProject
	}
	for d := dir; !isHome(d); {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return dir, nil
}

// AgentHooksStatus is what an agent's hook file says about the ShellTime hooks in it.
type AgentHooksStatus struct {
	// Installed is true when the file registers ShellTime for at least one event.
	Installed bool
	// MissingEvents are the managed events ShellTime isn't registered for, in install order.
	MissingEvents []string
	// Binaries are the shelltime binaries the registered commands run, each once.
	Binaries []string
	// EventBinaries are the binaries each managed event's commands run, by event.
	EventBinaries map[string][]string
}

// addBinary records bin as run by event's commands.
func (s *AgentHooksStatus) addBinary(event, bin string) {
	if bin == "" {
		return
	}
	if s.EventBinaries == nil {
		s.EventBinaries = map[string][]string{}
	}
	if !containsString(s.EventBinaries[event], bin) {
		s.EventBinaries[event] = append(s.EventBinaries[event], bin)
	}
	if !containsString(s.Binaries, bin) {
		s.Binaries = append(s.Binaries, bin)
	}
}

func containsString(values []string, s string) bool {
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}

// hookCommandBinary returns the binary a hook command runs hook-ingest with: the text before
// " hook-ingest" (after "out=$(" or "&& exec " in a project hook command), unquoted. Empty when
// the command doesn't run hook-ingest. Older versions wrote `<absolute path> hook-ingest …`,
// quoted when the shell would split the path.
func hookCommandBinary(command string) string {
	before, _, ok := strings.Cut(command, " "+HookIngestSubcommand)
	if !ok {
		return ""
	}
	for _, prefix := range []string{"out=$(", "&& exec "} {
		if i := strings.LastIndex(before, prefix); i >= 0 {
			before = before[i+len(prefix):]
			break
		}
	}
	bin := strings.TrimSpace(before)
	switch {
	case len(bin) >= 2 && bin[0] == '\'' && bin[len(bin)-1] == '\'':
		return strings.ReplaceAll(bin[1:len(bin)-1], `'\''`, "'")
	case len(bin) >= 2 && bin[0] == '"' && bin[len(bin)-1] == '"':
		return bin[1 : len(bin)-1]
	}
	return bin
}

// writeHookFile replaces an agent's hook file with data, creating its folder (0755) when
// missing. A hook file that is a symlink (kept in a dotfiles repository) is written through,
// not replaced.
func writeHookFile(path string, data []byte, mode os.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(path), err)
	}
	return writeFileAtomicMode(path, data, mode)
}

// marshalJSONValue encodes v without escaping <, > and &, which a command line may hold.
func marshalJSONValue(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// commandField returns the "command" string of a hook entry object; ok is false for anything
// that isn't an object with a string command.
func commandField(entry json.RawMessage) (fields []jsonField, command string, ok bool) {
	trimmed := bytes.TrimSpace(entry)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, "", false
	}
	fields, err := decodeOrderedObject(entry)
	if err != nil {
		return nil, "", false
	}
	raw, found := getField(fields, "command")
	if !found {
		return fields, "", false
	}
	if err := json.Unmarshal(raw, &command); err != nil {
		return fields, "", false
	}
	return fields, command, true
}

// decodeJSONArray decodes a JSON array into its raw elements. null is an empty array.
func decodeJSONArray(data json.RawMessage) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func encodeJSONArray(items []json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(item)
	}
	buf.WriteByte(']')
	return buf.Bytes()
}
