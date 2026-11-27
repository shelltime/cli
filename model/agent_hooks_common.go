package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// HookIngestSubcommand is what every agent hook command runs: `<bin> hook-ingest --agent …`.
const HookIngestSubcommand = "hook-ingest"

const (
	// ProjectHookBinary is the binary a project's hook file runs. The file is committed and
	// runs on every machine that checks the repository out, a cloud agent's VM included, so it
	// names shelltime as each machine's PATH finds it, never this machine's path.
	ProjectHookBinary = "shelltime"
	// ProjectHookFlag marks a hook command written to a project's hook file. The agent runs
	// those next to the user's own hooks, so hook-ingest leaves an event to the user's hook when
	// that records it too.
	ProjectHookFlag = "--project"
)

// ProjectHooksRoot returns the root of the project dir is in, where its agents look for project
// hook files: the nearest folder up from dir that holds .git (a folder, or a file in a worktree
// or submodule), else dir itself.
func ProjectHooksRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir, nil
		}
		d = parent
	}
}

// AgentHooksStatus is what an agent's hook file says about the ShellTime hooks in it.
type AgentHooksStatus struct {
	// Installed is true when the file registers ShellTime for at least one event.
	Installed bool
	// MissingEvents are the managed events ShellTime isn't registered for, in install order.
	MissingEvents []string
	// Binaries are the shelltime binaries the registered commands run, each once.
	Binaries []string
}

func (s *AgentHooksStatus) addBinary(bin string) {
	if bin == "" {
		return
	}
	for _, b := range s.Binaries {
		if b == bin {
			return
		}
	}
	s.Binaries = append(s.Binaries, bin)
}

// Seams for HookBinaryPath; tests replace them.
var (
	hookExecutable = os.Executable
	hookLookPath   = exec.LookPath
)

// HookBinaryPath returns the shelltime binary agent hooks should run: the running one, under
// the name PATH finds it by when that is the same file, so Homebrew's stable bin/shelltime is
// written rather than a versioned Cellar path that the next upgrade removes.
func HookBinaryPath() (string, error) {
	exe, err := hookExecutable()
	if err != nil {
		return "", err
	}
	if p, err := hookLookPath("shelltime"); err == nil {
		if abs, err := filepath.Abs(p); err == nil && sameFile(abs, exe) {
			return abs, nil
		}
	}
	return exe, nil
}

// hookCommand returns the hook command line that runs bin with args, quoting bin when the
// shell would split or expand it.
func hookCommand(bin string, args ...string) string {
	return strings.Join(append([]string{quoteHookBinary(bin)}, args...), " ")
}

func quoteHookBinary(bin string) string {
	safe := true
	for _, r := range bin {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("/._-+:@%,=", r):
		case r == '\\' && runtime.GOOS == "windows":
		default:
			safe = false
		}
	}
	if safe && bin != "" {
		return bin
	}
	if runtime.GOOS == "windows" {
		return `"` + bin + `"`
	}
	return "'" + strings.ReplaceAll(bin, "'", `'\''`) + "'"
}

// hookCommandBinary returns the binary of a hook command written by hookCommand: the text
// before " hook-ingest", unquoted. Empty when the command doesn't run hook-ingest.
func hookCommandBinary(command string) string {
	before, _, ok := strings.Cut(command, " "+HookIngestSubcommand)
	if !ok {
		return ""
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
