package model

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Text limits for backfilled events. They keep a 500-event batch well under
// the server's body limit even when prompts contain pasted files.
const (
	backfillMaxPromptBytes   = 16 << 10
	backfillMaxErrorBytes    = 1 << 10
	backfillMaxToolArgsBytes = 2 << 10
)

// backfillSettleWindow keeps sessions that were active very recently out of
// a backfill: their transcripts may still grow, and since uploads are
// idempotent per event id, a partial usage line would win forever.
const backfillSettleWindow = 30 * time.Minute

// BackfillOptions controls which local sessions a backfill picks up.
type BackfillOptions struct {
	Since     time.Time // sessions starting before this are skipped (zero: no bound)
	Until     time.Time // sessions starting at or after this are skipped (zero: no bound)
	NoPrompts bool      // upload prompt lengths but not the prompt text
	Now       time.Time
}

// BackfillSession is one Claude Code or Codex session rebuilt from local
// transcripts, with its events sorted by time.
type BackfillSession struct {
	ID         string
	ClientType string
	Start      time.Time
	End        time.Time
	Events     []AICodeBackfillEvent
}

// BackfillParseResult is what a transcript parser found on disk.
type BackfillParseResult struct {
	Sessions []*BackfillSession
	Files    int
	BadLines int
}

// backfillHash derives both the event id and the in-memory dedup key from a
// natural key of the event (message id, tool call id, ...). The session id is
// deliberately not part of it, so copies of the same item in resumed or
// forked transcripts collapse into one event.
func backfillHash(clientType, naturalKey string) [sha256.Size]byte {
	return sha256.Sum256([]byte(clientType + "\x00" + naturalKey))
}

func backfillEventID(clientType, naturalKey string) string {
	sum := backfillHash(clientType, naturalKey)
	return "bf1:" + hex.EncodeToString(sum[:20])
}

type backfillKey [16]byte

func newBackfillKey(clientType, naturalKey string) backfillKey {
	sum := backfillHash(clientType, naturalKey)
	var k backfillKey
	copy(k[:], sum[:16])
	return k
}

func newBackfillEvent(clientType, eventType, naturalKey, sessionID string, ts time.Time) AICodeBackfillEvent {
	return AICodeBackfillEvent{
		EventID:    backfillEventID(clientType, naturalKey),
		EventType:  eventType,
		ClientType: clientType,
		Timestamp:  ts.Unix(),
		SessionID:  sessionID,
	}
}

// backfillMachine describes this machine the same way the live OTEL resource
// attributes do (see claudeSettingsResourceAttributes).
type backfillMachine struct {
	OSType      string
	HostArch    string
	UserName    string
	MachineName string
}

func currentBackfillMachine() backfillMachine {
	userName, machineName := aiCodeResourceIdentity()
	return backfillMachine{
		OSType:      runtime.GOOS,
		HostArch:    runtime.GOARCH,
		UserName:    userName,
		MachineName: machineName,
	}
}

func (m backfillMachine) apply(e *AICodeBackfillEvent) {
	e.OSType = m.OSType
	e.HostArch = m.HostArch
	e.UserName = m.UserName
	e.MachineName = m.MachineName
	e.TeamID = "shelltime"
}

// readJSONLLines calls fn for every non-blank line of a JSONL file. Lines
// can be several megabytes (Claude Code stores tool results inline), so no
// line length limit is applied. fn owns the slice it receives.
func readJSONLLines(path string, fn func(line []byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")
		if len(bytes.TrimSpace(line)) > 0 {
			fn(line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// collectJSONL lists every *.jsonl file under the given roots, sorted.
// Files last modified more than a day before since cannot hold sessions in
// range and are skipped. Missing roots are ignored.
func collectJSONL(roots []string, since time.Time) ([]string, error) {
	seenRoots := map[string]bool{}
	seenFiles := map[string]bool{}
	var files []string

	for _, root := range roots {
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if seenRoots[resolved] {
			continue
		}
		seenRoots[resolved] = true

		err = filepath.WalkDir(resolved, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				// Unreadable directory: skip it rather than abort the backfill.
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			if !since.IsZero() {
				info, err := d.Info()
				if err == nil && info.ModTime().Before(since.Add(-24*time.Hour)) {
					return nil
				}
			}
			if !seenFiles[path] {
				seenFiles[path] = true
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}

// splitPathList splits a comma-separated list of paths, as used by the
// CLAUDE_CONFIG_DIR and CODEX_HOME environment variables.
func splitPathList(value string) []string {
	var out []string
	for _, p := range strings.Split(value, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SelectBackfillSessions keeps the sessions that start within the requested
// range and are not still active. It returns them and how many active
// sessions were held back.
func SelectBackfillSessions(sessions []*BackfillSession, opts BackfillOptions) ([]*BackfillSession, int) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	settled := now.Add(-backfillSettleWindow)

	var kept []*BackfillSession
	active := 0
	for _, s := range sessions {
		if len(s.Events) == 0 {
			continue
		}
		if !opts.Since.IsZero() && s.Start.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && !s.Start.Before(opts.Until) {
			continue
		}
		if s.End.After(settled) {
			active++
			continue
		}
		kept = append(kept, s)
	}
	return kept, active
}

// PackBackfillBatches groups sessions into upload requests bounded by
// maxEvents events, maxBytes of encoded events and maxCompleted completed
// sessions. Whole sessions are kept together when they fit; a larger session
// is split, and a session is listed in completedSessionIds only on the
// request carrying its last event, so the server builds its summary once all
// of its events are stored.
func PackBackfillBatches(clientType string, sessions []*BackfillSession, maxEvents, maxBytes, maxCompleted int) []AICodeBackfillRequest {
	ordered := make([]*BackfillSession, len(sessions))
	copy(ordered, sessions)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].Start.Equal(ordered[j].Start) {
			return ordered[i].Start.Before(ordered[j].Start)
		}
		return ordered[i].ID < ordered[j].ID
	})

	var batches []AICodeBackfillRequest
	cur := AICodeBackfillRequest{ClientType: clientType}
	curBytes := 0
	flush := func() {
		if len(cur.Events) > 0 || len(cur.CompletedSessionIDs) > 0 {
			batches = append(batches, cur)
		}
		cur = AICodeBackfillRequest{ClientType: clientType}
		curBytes = 0
	}

	for _, s := range ordered {
		sizes := make([]int, len(s.Events))
		sessionBytes := 0
		for i := range s.Events {
			sizes[i] = backfillEventSize(&s.Events[i])
			sessionBytes += sizes[i]
		}

		// Start a fresh request rather than split a session that would fit
		// into one on its own.
		if len(cur.CompletedSessionIDs) >= maxCompleted ||
			(len(cur.Events) > 0 && (len(cur.Events)+len(s.Events) > maxEvents || curBytes+sessionBytes > maxBytes)) {
			flush()
		}
		for i, e := range s.Events {
			if len(cur.Events) > 0 && (len(cur.Events) >= maxEvents || curBytes+sizes[i] > maxBytes) {
				flush()
			}
			cur.Events = append(cur.Events, e)
			curBytes += sizes[i]
		}
		cur.CompletedSessionIDs = append(cur.CompletedSessionIDs, s.ID)
	}
	flush()
	return batches
}

func backfillEventSize(e *AICodeBackfillEvent) int {
	buf, err := json.Marshal(e)
	if err != nil {
		return 0
	}
	return len(buf) + 1
}

// finalizeBackfillSession sorts a session's events and sets its time range.
func finalizeBackfillSession(s *BackfillSession) {
	sort.SliceStable(s.Events, func(i, j int) bool {
		return s.Events[i].Timestamp < s.Events[j].Timestamp
	})
	if len(s.Events) == 0 {
		return
	}
	first := time.Unix(s.Events[0].Timestamp, 0)
	last := time.Unix(s.Events[len(s.Events)-1].Timestamp, 0)
	if s.Start.IsZero() || first.Before(s.Start) {
		s.Start = first
	}
	if s.End.IsZero() || last.After(s.End) {
		s.End = last
	}
}
