package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// m2setupHome resets the storage globals to a fresh temp HOME and returns it.
func m2setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	InitFolder("") // reset COMMAND_* globals under the new HOME
	return home
}

func m2cmd(command string, when time.Time) Command {
	return Command{
		Shell:     "bash",
		SessionID: 1,
		Command:   command,
		Username:  "tester",
		Hostname:  "h",
		Time:      when,
	}
}

// --- fileStore additional branches ---

// TestFileStore_EmptyGetters covers the file store readers when nothing has
// been stored yet (storage folder created, files empty/missing).
func TestFileStore_EmptyGetters(t *testing.T) {
	m2setupHome(t)
	require.NoError(t, ensureStorageFolder())

	store := NewFileStore()
	ctx := context.Background()

	// Cursor on a fresh store: no cursor recorded.
	_, noCursor, err := store.GetLastCursor(ctx)
	require.NoError(t, err)
	assert.True(t, noCursor)

	// Post commands: empty file present -> empty slice, no error.
	require.NoError(t, os.WriteFile(GetPostCommandFilePath(), []byte(""), 0o644))
	posts, err := store.GetPostCommands(ctx)
	require.NoError(t, err)
	assert.Empty(t, posts)

	require.NoError(t, store.Close()) // no-op for file store
	assert.Equal(t, StorageEngineFile, store.Engine())
}

// TestFileStore_GetPostCommands_SkipsInvalidLines ensures malformed lines are
// skipped while valid records parse (covers the FromLineBytes error branch).
func TestFileStore_GetPostCommands_SkipsInvalidLines(t *testing.T) {
	m2setupHome(t)
	require.NoError(t, ensureStorageFolder())

	store := newFileStore()
	ctx := context.Background()
	now := time.Now()

	// Write one valid post line via the store, then append a garbage line.
	post := m2cmd("ls", now)
	require.NoError(t, store.SavePost(ctx, post, 0, now))

	f, err := os.OpenFile(GetPostCommandFilePath(), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("not-a-valid-command-line\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	posts, err := store.GetPostCommands(ctx)
	require.NoError(t, err)
	require.Len(t, posts, 1, "garbage line skipped, valid one kept")
	assert.Equal(t, "ls", posts[0].Command)
}

// TestFileStore_Prune_NoPostShortCircuits hits the "no post commands -> return
// nil early" branch of Prune.
func TestFileStore_Prune_NoPostShortCircuits(t *testing.T) {
	m2setupHome(t)
	require.NoError(t, ensureStorageFolder())

	store := newFileStore()
	ctx := context.Background()

	// Empty post file -> Prune returns immediately without touching pre.
	require.NoError(t, os.WriteFile(GetPostCommandFilePath(), []byte(""), 0o644))
	require.NoError(t, store.Prune(ctx, time.Now()))
}

// TestFileStore_Prune_KeepsUnfinishedPre stores a finished+synced command and a
// separate unfinished pre, then prunes: the unfinished pre survives.
func TestFileStore_Prune_KeepsUnfinishedPre(t *testing.T) {
	m2setupHome(t)
	require.NoError(t, ensureStorageFolder())

	store := newFileStore()
	ctx := context.Background()
	base := time.Now()

	finished := m2cmd("make build", base)
	require.NoError(t, store.SavePre(ctx, finished, base))
	require.NoError(t, store.SavePost(ctx, finished, 0, base))

	// Unfinished pre (no matching post) at the same recording time.
	unfinished := m2cmd("tail -f log", base)
	require.NoError(t, store.SavePre(ctx, unfinished, base))

	// A post after the cursor that must be kept.
	later := base.Add(time.Hour)
	require.NoError(t, store.SavePost(ctx, m2cmd("echo later", later), 0, later))

	require.NoError(t, store.SetCursor(ctx, base))
	require.NoError(t, store.Prune(ctx, base))

	pres, err := store.GetPreCommands(ctx)
	require.NoError(t, err)
	cmds := make([]string, 0, len(pres))
	for _, c := range pres {
		cmds = append(cmds, c.Command)
	}
	assert.Contains(t, cmds, "tail -f log", "unfinished pre kept")

	posts, err := store.GetPostCommands(ctx)
	require.NoError(t, err)
	var keptLater bool
	for _, p := range posts {
		if p.Command == "echo later" {
			keptLater = true
		}
	}
	assert.True(t, keptLater, "post after cursor kept")
}

// --- boltStore additional branches ---

// TestBoltStore_ReopenExisting covers newBoltStore on an already-initialized DB
// file (buckets already exist) plus Close on an already-closed handle.
func TestBoltStore_ReopenExisting(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "commands.db")

	st1, err := newBoltStore(dbPath)
	require.NoError(t, err)
	require.NoError(t, st1.SavePre(context.Background(), m2cmd("a", time.Now()), time.Now()))
	require.NoError(t, st1.Close())

	// Reopen the same file: buckets already present, must succeed.
	st2, err := newBoltStore(dbPath)
	require.NoError(t, err)
	pre, err := st2.GetPreCommands(context.Background())
	require.NoError(t, err)
	assert.Len(t, pre, 1)
	require.NoError(t, st2.Close())
}

// TestBoltStore_CloseNilDB covers the Close guard when db is nil.
func TestBoltStore_CloseNilDB(t *testing.T) {
	s := &boltStore{db: nil}
	assert.NoError(t, s.Close())
}

// TestBoltStore_Prune_NothingToDelete runs Prune against an empty DB so both
// ForEach passes iterate over nothing and no deletes happen.
func TestBoltStore_Prune_NothingToDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "commands.db")
	st, err := newBoltStore(dbPath)
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.Prune(context.Background(), time.Now()))

	pre, err := st.GetPreCommands(context.Background())
	require.NoError(t, err)
	assert.Empty(t, pre)
}

// TestDecodeKeyNano_ShortKey covers the len < 8 guard returning 0.
func TestDecodeKeyNano_ShortKey(t *testing.T) {
	assert.Equal(t, int64(0), decodeKeyNano([]byte{1, 2, 3}))
	assert.Equal(t, int64(0), decodeKeyNano(nil))
}

// --- db.go package-level reader branches ---

// TestDB_GetPreCommands_FileNotExists covers the open-error branch.
func TestDB_GetPreCommands_FileNotExists(t *testing.T) {
	m2setupHome(t)
	require.NoError(t, ensureStorageFolder())
	// pre.txt does not exist -> error returned.
	_, err := GetPreCommands(context.Background())
	assert.Error(t, err)
}

// TestDB_GetLastCursor_BlankLinesOnly covers the "lastLine == ''" path: the file
// holds only blank lines, so the zero cursor is returned with no error.
func TestDB_GetLastCursor_BlankLinesOnly(t *testing.T) {
	m2setupHome(t)
	require.NoError(t, ensureStorageFolder())
	require.NoError(t, os.WriteFile(GetCursorFilePath(), []byte("\n\n"), 0o644))

	cursor, noCursor, err := GetLastCursor(context.Background())
	require.NoError(t, err)
	assert.False(t, noCursor, "file exists, so noCursorExist stays false")
	assert.True(t, cursor.IsZero())
}

// TestDB_GetPreCommandsTree_SkipsInvalidLines feeds a mix of a valid line and a
// malformed one and asserts only the valid one lands in the tree.
func TestDB_GetPreCommandsTree_SkipsInvalidLines(t *testing.T) {
	m2setupHome(t)
	require.NoError(t, ensureStorageFolder())

	store := newFileStore()
	ctx := context.Background()
	now := time.Now()
	valid := m2cmd("git status", now)
	require.NoError(t, store.SavePre(ctx, valid, now))

	f, err := os.OpenFile(GetPreCommandFilePath(), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("garbage-line-here\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	tree, err := GetPreCommandsTree(ctx)
	require.NoError(t, err)
	require.Len(t, tree[valid.GetUniqueKey()], 1)
}
