package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testBackfillSession(id string, start time.Time, events int) *BackfillSession {
	s := &BackfillSession{ID: id, ClientType: AICodeClientClaudeCode}
	for i := 0; i < events; i++ {
		ts := start.Add(time.Duration(i) * time.Second)
		s.Events = append(s.Events, newBackfillEvent(AICodeClientClaudeCode, AICodeEventApiRequest, fmt.Sprintf("%s-%d", id, i), id, ts))
	}
	finalizeBackfillSession(s)
	return s
}

func TestBackfillEventIDIsStable(t *testing.T) {
	// Event ids must never change between CLI versions, or re-running a
	// backfill would upload everything twice.
	assert.Equal(t, "bf1:efb74f1d4dd4e799fe39035fce73f1d5da2c3a5c", backfillEventID(AICodeClientClaudeCode, "msg_1:req_1"))
	assert.Regexp(t, `^bf1:[0-9a-f]{40}$`, backfillEventID(AICodeClientCodex, "call_1"))
	assert.NotEqual(t, backfillEventID(AICodeClientClaudeCode, "x"), backfillEventID(AICodeClientCodex, "x"))
	assert.Equal(t, newBackfillKey(AICodeClientCodex, "x"), newBackfillKey(AICodeClientCodex, "x"))
}

func TestPackBackfillBatches(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	sessions := []*BackfillSession{
		testBackfillSession("c", base.Add(2*time.Hour), 3),
		testBackfillSession("a", base, 4),
		testBackfillSession("big", base.Add(time.Hour), 12),
	}

	batches := PackBackfillBatches(AICodeClientClaudeCode, sessions, 5, AICodeBackfillMaxBatchBytes, 50)

	// a (4) fits alone; big (12) is split 5+5+2; c (3) joins big's last chunk.
	require.Len(t, batches, 4)
	assert.Len(t, batches[0].Events, 4)
	assert.Equal(t, []string{"a"}, batches[0].CompletedSessionIDs)
	assert.Len(t, batches[1].Events, 5)
	assert.Empty(t, batches[1].CompletedSessionIDs)
	assert.Len(t, batches[2].Events, 5)
	assert.Empty(t, batches[2].CompletedSessionIDs)
	assert.Len(t, batches[3].Events, 5)
	assert.Equal(t, []string{"big", "c"}, batches[3].CompletedSessionIDs)
	for _, b := range batches {
		assert.Equal(t, AICodeClientClaudeCode, b.ClientType)
		assert.LessOrEqual(t, len(b.Events), 5)
	}
}

func TestPackBackfillBatchesRespectsCompletedCap(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	var sessions []*BackfillSession
	for i := 0; i < 5; i++ {
		sessions = append(sessions, testBackfillSession(fmt.Sprintf("s%d", i), base.Add(time.Duration(i)*time.Minute), 1))
	}

	batches := PackBackfillBatches(AICodeClientCodex, sessions, 500, AICodeBackfillMaxBatchBytes, 2)

	require.Len(t, batches, 3)
	assert.Equal(t, []string{"s0", "s1"}, batches[0].CompletedSessionIDs)
	assert.Equal(t, []string{"s2", "s3"}, batches[1].CompletedSessionIDs)
	assert.Equal(t, []string{"s4"}, batches[2].CompletedSessionIDs)
}

func TestPackBackfillBatchesRespectsByteBudget(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	s := testBackfillSession("prompts", base, 6)
	for i := range s.Events {
		s.Events[i].Prompt = strings.Repeat("p", 1000)
	}
	eventSize := backfillEventSize(&s.Events[0])

	batches := PackBackfillBatches(AICodeClientClaudeCode, []*BackfillSession{s}, 500, eventSize*2, 50)

	require.Len(t, batches, 3)
	for _, b := range batches {
		assert.Len(t, b.Events, 2)
	}
	assert.Equal(t, []string{"prompts"}, batches[2].CompletedSessionIDs)
}

func TestSelectBackfillSessions(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	old := testBackfillSession("old", now.AddDate(0, 0, -20), 2)
	mid := testBackfillSession("mid", now.AddDate(0, 0, -5), 2)
	active := testBackfillSession("active", now.Add(-10*time.Minute), 2)
	empty := &BackfillSession{ID: "empty"}

	kept, activeCount := SelectBackfillSessions([]*BackfillSession{old, mid, active, empty}, BackfillOptions{Now: now})
	assert.Equal(t, []*BackfillSession{old, mid}, kept)
	assert.Equal(t, 1, activeCount)

	kept, _ = SelectBackfillSessions([]*BackfillSession{old, mid}, BackfillOptions{Now: now, Since: now.AddDate(0, 0, -10)})
	assert.Equal(t, []*BackfillSession{mid}, kept)

	kept, _ = SelectBackfillSessions([]*BackfillSession{old, mid}, BackfillOptions{Now: now, Until: now.AddDate(0, 0, -10)})
	assert.Equal(t, []*BackfillSession{old}, kept)
}

func TestReadJSONLLinesHandlesHugeLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.jsonl")
	huge := `{"x":"` + strings.Repeat("a", 10<<20) + `"}`
	require.NoError(t, os.WriteFile(path, []byte("{\"a\":1}\r\n\n"+huge+"\n{\"b\":2}"), 0o644))

	var lens []int
	require.NoError(t, readJSONLLines(path, func(line []byte) { lens = append(lens, len(line)) }))

	assert.Equal(t, []int{7, len(huge), 7}, lens)
}

func TestCollectJSONL(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "proj", "sub"), 0o755))
	recent := filepath.Join(root, "proj", "recent.jsonl")
	stale := filepath.Join(root, "proj", "sub", "stale.jsonl")
	require.NoError(t, os.WriteFile(recent, []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(stale, []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "proj", "notes.txt"), []byte("x"), 0o644))
	staleTime := time.Now().AddDate(0, 0, -30)
	require.NoError(t, os.Chtimes(stale, staleTime, staleTime))

	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(root, link))

	all, err := collectJSONL([]string{root, link, filepath.Join(root, "missing")}, time.Time{})
	require.NoError(t, err)
	assert.Len(t, all, 2)

	inRange, err := collectJSONL([]string{root}, time.Now().AddDate(0, 0, -7))
	require.NoError(t, err)
	assert.Len(t, inRange, 1)
	assert.Equal(t, "recent.jsonl", filepath.Base(inRange[0]))
}
