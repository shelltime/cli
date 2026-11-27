package model

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/malamtime/cli/agenthook"
)

// agentHookPendingMu guards the pending file within a process: the daemon appends what it
// couldn't send while its resync service reads and rewrites the file. Other processes
// (hook-ingest without a daemon) only append, a line per write, which O_APPEND keeps whole.
var agentHookPendingMu sync.Mutex

// AppendAgentHookPending appends events to the agent hook pending file, one JSON line each
// and one write per line, so lines appended by other processes never interleave with them.
func AppendAgentHookPending(events ...agenthook.Event) error {
	if len(events) == 0 {
		return nil
	}
	agentHookPendingMu.Lock()
	defer agentHookPendingMu.Unlock()

	path := GetAgentHooksPendingFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer f.Close()

	for _, ev := range events {
		line, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("failed to encode agent hook event: %w", err)
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}
	return nil
}

// ReadAgentHookPendingLines returns the non-empty lines of the agent hook pending file. A
// missing file returns an error satisfying os.IsNotExist.
func ReadAgentHookPendingLines() ([]string, error) {
	agentHookPendingMu.Lock()
	defer agentHookPendingMu.Unlock()
	return readAgentHookPendingLines(GetAgentHooksPendingFilePath())
}

// UpdateAgentHookPending replaces the pending file's lines with what update returns for the
// lines it holds now (none when it is missing). No line left removes the file.
func UpdateAgentHookPending(update func(current []string) []string) error {
	agentHookPendingMu.Lock()
	defer agentHookPendingMu.Unlock()

	path := GetAgentHooksPendingFilePath()
	current, err := readAgentHookPendingLines(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return rewriteAgentHookPending(path, update(current))
}

func readAgentHookPendingLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// An event holds up to a few hundred KB of text, so lines are not size limited
	var lines []string
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if line = strings.TrimRight(line, "\r\n"); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
		if err == io.EOF {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func rewriteAgentHookPending(path string, lines []string) error {
	if len(lines) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmp.Name()) // fails harmlessly once renamed

	w := bufio.NewWriter(tmp)
	for _, line := range lines {
		if _, err := w.WriteString(line + "\n"); err != nil {
			tmp.Close()
			return fmt.Errorf("failed to write temp file: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0600); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	return nil
}
