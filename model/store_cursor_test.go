package model

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storedCursor(t *testing.T, store *fileStore) time.Time {
	t.Helper()
	cursor, noCursor, err := store.GetLastCursor(context.Background())
	require.NoError(t, err)
	require.False(t, noCursor)
	return cursor
}

func waitingCommands(t *testing.T, store *fileStore) []string {
	t.Helper()
	res, err := BuildTrackingData(context.Background(), store, ShellTimeConfig{})
	require.NoError(t, err)
	names := make([]string, 0, len(res.Data))
	for _, data := range res.Data {
		names = append(names, data.Command)
	}
	return names
}

// Two processes sync the same store: the daemon after a batch, and `shelltime sync` from a
// shell, which covered more. When the daemon then prunes with the cursor of its own, older
// batch, it must not put that one back: the commands in between would be sent again.
func TestPrune_NeverMovesTheCursorBackwards(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	first := savePair(t, store, "git status", 0)
	second := savePair(t, store, "make build", time.Minute)
	savePair(t, store, "go test ./...", 2*time.Minute)

	// The other process synced up to the second command
	require.NoError(t, store.SetCursor(ctx, second))

	// This one only got as far as the first
	require.NoError(t, store.Prune(ctx, first))

	assert.Equal(t, second.UnixNano(), storedCursor(t, store).UnixNano(), "the later cursor stays")
	assert.Equal(t, []string{"go test ./..."}, waitingCommands(t, store), "what the later sync covered is not waiting again")
	assert.Equal(t, []string{"go test ./..."}, preCommandNames(t, store), "and is pruned as synced")
}

// The daemon's own order is SetCursor, then Prune with the same time. When its batch was
// the older one, the SetCursor already appended an older time after the later one.
func TestSetCursor_AnOlderCursorDoesNotReplaceALaterOne(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	first := savePair(t, store, "git status", 0)
	second := savePair(t, store, "make build", time.Minute)
	savePair(t, store, "go test ./...", 2*time.Minute)

	require.NoError(t, store.SetCursor(ctx, second))
	require.NoError(t, store.SetCursor(ctx, first))

	assert.Equal(t, second.UnixNano(), storedCursor(t, store).UnixNano())
	assert.Equal(t, []string{"go test ./..."}, waitingCommands(t, store))

	// Either compaction settles the file on the later one
	compactions := map[string]func() error{
		"daemon prune": func() error { return store.Prune(ctx, first) },
		"gc":           func() error { return CompactCommandFiles(ctx) },
	}
	for name, compact := range compactions {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(GetCursorFilePath(), []byte(fmt.Sprintf("\n%d\n\n%d\n", second.UnixNano(), first.UnixNano())), 0o600))
			require.NoError(t, compact())

			content, err := os.ReadFile(GetCursorFilePath())
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("%d", second.UnixNano()), string(content))
			assert.Equal(t, []string{"go test ./..."}, waitingCommands(t, store))
		})
	}
}

// A prune that knows more than the store still moves the cursor forward.
func TestPrune_MovesTheCursorForward(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	first := savePair(t, store, "git status", 0)
	second := savePair(t, store, "make build", time.Minute)
	require.NoError(t, store.SetCursor(ctx, first))

	require.NoError(t, store.Prune(ctx, second))

	assert.Equal(t, second.UnixNano(), storedCursor(t, store).UnixNano())
	assert.Empty(t, waitingCommands(t, store))
}

func TestParseLastCursor(t *testing.T) {
	at := func(nano int64) time.Time { return time.Unix(0, nano) }
	tests := []struct {
		name    string
		content string
		want    time.Time
		found   bool
		wantErr bool
	}{
		{name: "empty", content: ""},
		{name: "blank lines only", content: "\n\n"},
		{name: "one line", content: "100", want: at(100), found: true},
		{name: "as SetCursor appends", content: "\n100\n\n200\n", want: at(200), found: true},
		{name: "an older time appended last", content: "\n200\n\n100\n", want: at(200), found: true},
		{name: "the latest in the middle", content: "100\n300\n200\n", want: at(300), found: true},
		{name: "an older line that is no number is passed over", content: "junk\n100\n", want: at(100), found: true},
		{name: "a last line that is no number is an error", content: "100\njunk\n", wantErr: true},
		{name: "only a line that is no number", content: "not-a-number", wantErr: true},
		{name: "before 1970", content: "-200\n-100\n", want: at(-100), found: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := parseLastCursor([]byte(tt.content))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.found, found)
			if tt.found {
				assert.Equal(t, tt.want.UnixNano(), got.UnixNano())
			} else {
				assert.True(t, got.IsZero())
			}
		})
	}
}

// A cursor.txt the store can't read is replaced by a prune's cursor, as it always was.
func TestPrune_ReplacesAnUnreadableCursor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	first := savePair(t, store, "git status", 0)
	require.NoError(t, os.WriteFile(GetCursorFilePath(), []byte("not-a-number\n"), 0o600))

	require.NoError(t, store.Prune(ctx, first))
	assert.Equal(t, first.UnixNano(), storedCursor(t, store).UnixNano())
}
