package model

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/malamtime/cli/agenthook"
)

// The agent hook pending file (agent-hooks.pending.jsonl) is shared by processes that know
// nothing of each other: a hook-ingest per hook event when no daemon takes it, and the daemon,
// which appends what it couldn't send and whose resync service reads the file, sends it and
// rewrites it with the lines to keep. A line appended between that read and the rename that
// replaces the file would be gone.
//
// agent-hooks.pending.lock, next to the file, is an advisory lock (flock) against that, like
// the command store's store.lock: an append holds it shared, a rewrite holds it exclusively
// from reading the file to replacing it. Readers don't take it; a rewrite replaces the file
// with a rename, so they see it either as it was or as it is after. Within a process,
// agentHookPendingMu orders the same steps.
//
// Nobody waits for the lock longer than agentHookPendingLockWait. An append that couldn't get
// it appends anyway, and a rewrite carries over what it finds appended since its read, which
// leaves only the moment between that last look and the rename. A rewrite that couldn't get
// it changes nothing (ErrAgentHookPendingBusy): the next pass does the work. Where there is no
// lock at all (Windows, which has no daemon to rewrite the file, Solaris, a filesystem without
// flock), both go on unlocked.
var agentHookPendingMu sync.Mutex

// agentHookPendingLockWait bounds the wait for the pending file's lock. hook-ingest appends
// while the agent waits on the hook, and an append or a rewrite holds the lock for well under
// this, so only something stuck holds it longer.
var agentHookPendingLockWait = 50 * time.Millisecond

// ErrAgentHookPendingBusy is returned by UpdateAgentHookPending when another process held the
// pending file's lock for the whole wait. Nothing was changed.
var ErrAgentHookPendingBusy = errors.New("the agent hook pending file is locked by another process")

// agentHookPendingAfterRead runs in UpdateAgentHookPending after it read the file, under the
// lock; tests use it to append as an unlocked process would.
var agentHookPendingAfterRead func()

func agentHookPendingLockPath() string {
	return GetStoragePath("agent-hooks.pending.lock")
}

// AppendAgentHookPending appends events to the agent hook pending file, one JSON line each
// and one write per line, so lines appended by other processes never interleave with them.
func AppendAgentHookPending(events ...agenthook.Event) error {
	if len(events) == 0 {
		return nil
	}
	lines := make([][]byte, 0, len(events))
	for _, ev := range events {
		line, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("failed to encode agent hook event: %w", err)
		}
		lines = append(lines, append(line, '\n'))
	}

	agentHookPendingMu.Lock()
	defer agentHookPendingMu.Unlock()

	path := GetAgentHooksPendingFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(path), err)
	}
	// Without the lock the lines are appended all the same; a rewrite under way carries them over
	unlock, _ := acquireStoreLock(agentHookPendingLockPath(), false, agentHookPendingLockWait)
	defer unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer f.Close()
	for _, line := range lines {
		if _, err := f.Write(line); err != nil {
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
	snap, err := readAgentHookPending(GetAgentHooksPendingFilePath())
	return snap.lines, err
}

// UpdateAgentHookPending replaces the pending file's lines with what update returns for the
// lines it holds now (none when it is missing). Lines appended meanwhile by a process that
// went ahead without the lock are kept after them. No line left removes the file. When
// another process holds the lock for the whole wait, it returns ErrAgentHookPendingBusy
// without calling update.
func UpdateAgentHookPending(update func(current []string) []string) error {
	agentHookPendingMu.Lock()
	defer agentHookPendingMu.Unlock()

	unlock, state := acquireStoreLock(agentHookPendingLockPath(), true, agentHookPendingLockWait)
	defer unlock()
	if state == storeLockBusy {
		return ErrAgentHookPendingBusy
	}

	path := GetAgentHooksPendingFilePath()
	snap, err := readAgentHookPending(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if agentHookPendingAfterRead != nil {
		agentHookPendingAfterRead()
	}
	return rewriteAgentHookPending(path, update(snap.lines), snap)
}

// agentHookPendingSnapshot is the pending file as one read saw it: its lines, the file (nil
// when it was missing), how many bytes were read and the last of them.
type agentHookPendingSnapshot struct {
	lines []string
	info  os.FileInfo
	size  int64
	last  []byte
}

// agentHookPendingWindow is how many of the last bytes read a snapshot keeps to recognize the
// file later: a removed file's inode can be reused at once, so os.SameFile alone can't tell.
// It always holds part of an event's random id.
const agentHookPendingWindow = 4096

func readAgentHookPending(path string) (agentHookPendingSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return agentHookPendingSnapshot{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return agentHookPendingSnapshot{}, err
	}

	// An event holds up to a few hundred KB of text, so lines are not size limited
	snap := agentHookPendingSnapshot{info: info}
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		snap.size += int64(len(line))
		if line = strings.TrimRight(line, "\r\n"); strings.TrimSpace(line) != "" {
			snap.lines = append(snap.lines, line)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return agentHookPendingSnapshot{}, err
		}
	}
	snap.last = make([]byte, min(snap.size, agentHookPendingWindow))
	if _, err := f.ReadAt(snap.last, snap.size-int64(len(snap.last))); err != nil && err != io.EOF {
		return agentHookPendingSnapshot{}, err
	}
	return snap, nil
}

// appendedSince returns the whole lines in the file at path that read didn't see: the ones
// after what it read, or every line of a file that replaced it (log cleanup removed it, and an
// append made a new one).
func appendedSince(path string, read agentHookPendingSnapshot) ([]byte, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	from := int64(0)
	if read.info != nil && os.SameFile(read.info, info) && info.Size() >= read.size {
		// The file read, unless its bytes there changed: then another one took its inode
		last := make([]byte, len(read.last))
		if _, err := f.ReadAt(last, read.size-int64(len(last))); err != nil && err != io.EOF {
			return nil, err
		}
		if bytes.Equal(last, read.last) {
			from = read.size
		}
	}
	if info.Size() <= from {
		return nil, nil
	}
	data := make([]byte, info.Size()-from)
	n, err := f.ReadAt(data, from)
	if err != nil && err != io.EOF {
		return nil, err
	}
	// A line still being written is its writer's; without its end it isn't an event anyway
	data = data[:n]
	end := bytes.LastIndexByte(data, '\n')
	return data[:end+1], nil
}

func rewriteAgentHookPending(path string, lines []string, read agentHookPendingSnapshot) error {
	if len(lines) == 0 {
		tail, err := appendedSince(path, read)
		if err != nil {
			return err
		}
		if len(tail) == 0 {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove %s: %w", path, err)
			}
			return nil
		}
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
	// Looked at last, right before the rename, so as little as possible is appended after it
	tail, err := appendedSince(path, read)
	if err != nil {
		tmp.Close()
		return err
	}
	if _, err := w.Write(tail); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
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
