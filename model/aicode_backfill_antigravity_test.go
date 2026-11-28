package model

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pbVarint(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

func pbTag(field, wire int) []byte {
	return pbVarint(uint64(field<<3 | wire))
}

func pbIntField(field int, v int64) []byte {
	out := pbTag(field, 0)
	return append(out, pbVarint(uint64(v))...)
}

func pbBytesField(field int, payload []byte) []byte {
	out := pbTag(field, 2)
	out = append(out, pbVarint(uint64(len(payload)))...)
	return append(out, payload...)
}

func pbStringField(field int, s string) []byte {
	return pbBytesField(field, []byte(s))
}

func pbTimestampMsg(ts time.Time) []byte {
	var msg []byte
	msg = append(msg, pbIntField(1, ts.Unix())...)
	msg = append(msg, pbIntField(2, int64(ts.Nanosecond()))...)
	return msg
}

func TestParseAntigravityDBs_ProtobufAndDedup(t *testing.T) {
	root := t.TempDir()
	convDir := filepath.Join(root, "conversations")
	require.NoError(t, os.MkdirAll(convDir, 0755))

	// Non-conversation DB should be skipped cleanly.
	summaryDB, err := sql.Open("sqlite", filepath.Join(convDir, "conversation_summaries.db"))
	require.NoError(t, err)
	_, err = summaryDB.Exec(`CREATE TABLE summaries (id TEXT PRIMARY KEY)`)
	require.NoError(t, err)
	require.NoError(t, summaryDB.Close())

	// Conversation DB with both gen_metadata and steps referencing the same usage ID + a second distinct usage.
	ts := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(convDir, "agy-session-1.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`
		CREATE TABLE gen_metadata (idx INTEGER, data BLOB);
		CREATE TABLE steps (idx INTEGER, metadata BLOB);
		CREATE TABLE trajectory_metadata_blob (data BLOB);
	`)
	require.NoError(t, err)

	// Shared usage record with ID in field 11 so step & generation merge via union-find.
	var genUsage []byte
	genUsage = append(genUsage, pbIntField(1, 1036)...) // gemini-3.1-pro
	genUsage = append(genUsage, pbIntField(2, 1200)...) // uncached input
	genUsage = append(genUsage, pbIntField(3, 300)...)  // output
	genUsage = append(genUsage, pbIntField(4, 150)...)  // cache creation
	genUsage = append(genUsage, pbIntField(5, 4000)...) // cached input
	genUsage = append(genUsage, pbIntField(9, 100)...)  // reasoning
	genUsage = append(genUsage, pbIntField(10, 250)...) // visible output -> max(300, 350)=350
	genUsage = append(genUsage, pbStringField(11, "req-shared-1")...)

	var genData []byte
	genData = append(genData, pbIntField(3, 1036)...)
	genData = append(genData, pbBytesField(4, genUsage)...)
	genData = append(genData, pbBytesField(9, pbBytesField(4, pbTimestampMsg(ts)))...)
	genData = append(genData, pbStringField(19, "Gemini 3.1 Pro")...)
	genRoot := pbBytesField(1, genData)

	_, err = db.Exec(`INSERT INTO gen_metadata (idx, data) VALUES (0, ?)`, genRoot)
	require.NoError(t, err)

	// Step referencing the SAME usage ID ("req-shared-1") with lower tokens (should merge and keep max).
	var stepUsageDup []byte
	stepUsageDup = append(stepUsageDup, pbIntField(1, 1036)...)
	stepUsageDup = append(stepUsageDup, pbIntField(2, 1000)...)
	stepUsageDup = append(stepUsageDup, pbIntField(3, 200)...)
	stepUsageDup = append(stepUsageDup, pbStringField(11, "req-shared-1")...)

	var stepMeta1 []byte
	stepMeta1 = append(stepMeta1, pbBytesField(8, pbTimestampMsg(ts))...)
	stepMeta1 = append(stepMeta1, pbBytesField(9, stepUsageDup)...)
	stepMeta1 = append(stepMeta1, pbBytesField(24, pbIntField(1, 1036))...)
	_, err = db.Exec(`INSERT INTO steps (idx, metadata) VALUES (0, ?)`, stepMeta1)
	require.NoError(t, err)

	// Second step using model ID 1402 (fallback map -> claude-opus-5-5) and distinct usage ID.
	var stepUsage2 []byte
	stepUsage2 = append(stepUsage2, pbIntField(1, 1402)...)
	stepUsage2 = append(stepUsage2, pbIntField(2, 500)...)
	stepUsage2 = append(stepUsage2, pbIntField(3, 120)...)
	stepUsage2 = append(stepUsage2, pbStringField(11, "req-unique-2")...)

	var stepMeta2 []byte
	stepMeta2 = append(stepMeta2, pbBytesField(8, pbTimestampMsg(ts.Add(5*time.Minute)))...)
	stepMeta2 = append(stepMeta2, pbBytesField(9, stepUsage2)...)
	_, err = db.Exec(`INSERT INTO steps (idx, metadata) VALUES (1, ?)`, stepMeta2)
	require.NoError(t, err)

	require.NoError(t, db.Close())

	t.Setenv("ANTIGRAVITY_DATA_DIR", root)
	roots := AntigravityDataRoots()
	require.Len(t, roots, 1)
	assert.Equal(t, convDir, roots[0])

	res, err := ParseAntigravityDBs(roots, BackfillOptions{Now: ts.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, res.Sessions, 1)

	sess := res.Sessions[0]
	assert.Equal(t, "agy-session-1", sess.ID)
	assert.Equal(t, AICodeClientAntigravity, sess.ClientType)
	require.Len(t, sess.Events, 2)

	e0 := sess.Events[0]
	assert.Equal(t, "gemini-3.1-pro", e0.Model)
	assert.Equal(t, 1200, *e0.InputTokens)
	assert.Equal(t, 350, *e0.OutputTokens)
	assert.Equal(t, 4000, *e0.CacheReadTokens)
	assert.Equal(t, 150, *e0.CacheCreationTokens)
	require.NotNil(t, e0.ReasoningTokens)
	assert.Equal(t, 100, *e0.ReasoningTokens)

	e1 := sess.Events[1]
	assert.Equal(t, "claude-opus-5-5", e1.Model)
	assert.Equal(t, 500, *e1.InputTokens)
	assert.Equal(t, 120, *e1.OutputTokens)
}

func TestNormalizeAntigravityModelName(t *testing.T) {
	assert.Equal(t, "claude-sonnet-4-5", normalizeAntigravityModelName("Claude 4.5 Sonnet"))
	assert.Equal(t, "gemini-3.5-flash-high", normalizeAntigravityModelName("Gemini 3.5 Flash (High)"))
	assert.Equal(t, "gemini-3.1-pro", normalizeAntigravityModelName("Gemini 3.1 Pro"))
}
