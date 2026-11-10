package model

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// plainStore hides everything a store offers beyond CommandStore, so BuildTrackingData
// reads it the plain way: every post command decoded, then filtered by the cursor.
type plainStore struct{ CommandStore }

// randomStoreFiles writes a txt store of whatever shape the seed gives: commands in a few
// sessions that repeat, with and without their pre or post, synced and not, between lines
// no reader accepts, with every kind of cursor file.
func randomStoreFiles(t *testing.T, rng *rand.Rand) ShellTimeConfig {
	t.Helper()
	require.NoError(t, ensureStorageFolder())
	base := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	commands := []string{
		"ls", "git status", "make build", "export API_TOKEN=abc", "kubectl get pods",
		"curl -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U' x",
		"echo \"a\tb\"", "echo 'line1\nline2'", "",
	}
	junk := []string{
		"", "not a line", "{\"cmd\":\"no tab\"}", "{broken json\t1717243200000000000",
		"{\"shell\":\"zsh\",\"cmd\":\"x\"}\tnot-a-number", "{\"shell\":\"zsh\",\"cmd\":\"x\"}\t17\t18",
		"\t", "\t1717243200000000000", "{\"shell\":\"zsh\",\"cmd\":\"tabs\"}\t\t1717243200000000000",
	}

	var pre, post []byte
	var recordingTimes []time.Time
	addJunk := func(buf []byte) []byte {
		if rng.Intn(6) == 0 {
			buf = append(buf, junk[rng.Intn(len(junk))]...)
			buf = append(buf, '\n')
		}
		return buf
	}
	for i := range rng.Intn(45) {
		start := base.Add(time.Duration(rng.Intn(600)) * time.Second).Add(time.Duration(rng.Intn(1000)) * time.Millisecond)
		ssh := rng.Intn(2) == 0
		cmd := Command{
			Shell:     []string{"zsh", "bash"}[rng.Intn(2)],
			SessionID: int64(1 + rng.Intn(3)),
			Command:   commands[rng.Intn(len(commands))],
			Username:  "u",
			Hostname:  fmt.Sprintf("host-%d", rng.Intn(2)),
			Time:      start,
			PPID:      rng.Intn(3) * 1000,
		}
		if rng.Intn(3) == 0 {
			cmd.ViaSSH = &ssh
		}
		if rng.Intn(8) != 0 { // most commands have a pre
			line, err := cmd.ToLine(start)
			require.NoError(t, err)
			pre = addJunk(append(pre, line...))
		}
		if rng.Intn(6) != 0 { // most have finished
			end := cmd
			end.Phase = CommandPhasePost
			end.Result = rng.Intn(3)
			// A few "finish" before they start: a clock stepping back
			end.Time = start.Add(time.Duration(rng.Intn(5000)-300) * time.Millisecond)
			recorded := end.Time.Add(time.Duration(rng.Intn(3)) * time.Nanosecond)
			if i%7 == 0 && len(recordingTimes) > 0 {
				recorded = recordingTimes[rng.Intn(len(recordingTimes))] // two posts recorded at the same instant
			}
			recordingTimes = append(recordingTimes, recorded)
			line, err := end.ToLine(recorded)
			require.NoError(t, err)
			post = addJunk(append(post, line...))
		}
	}
	if len(post) > 0 && rng.Intn(5) == 0 {
		post = post[:len(post)-1] // no newline after the last line
	}

	if len(pre) > 0 || rng.Intn(4) != 0 {
		require.NoError(t, os.WriteFile(GetPreCommandFilePath(), pre, 0o600))
	}
	if len(post) > 0 || rng.Intn(4) != 0 {
		require.NoError(t, os.WriteFile(GetPostCommandFilePath(), post, 0o600))
	}

	var cursor string
	switch kind := rng.Intn(8); {
	case kind == 0: // no cursor file
	case kind == 1:
		cursor = "" // an empty one
	case kind == 2:
		cursor = "\nnot-a-cursor\n"
	case kind == 3:
		cursor = fmt.Sprintf("%d", base.Add(time.Hour).UnixNano()) // after everything
	case kind == 4:
		cursor = fmt.Sprintf("\n%d\n", base.Add(-time.Hour).UnixNano()) // before everything
	case len(recordingTimes) > 0:
		// A post's recording time, or a nanosecond to either side of it, after an older
		// cursor: the boundary between synced and waiting
		at := recordingTimes[rng.Intn(len(recordingTimes))].UnixNano() + int64(rng.Intn(3)-1)
		cursor = fmt.Sprintf("\n%d\n\n%d\n", base.UnixNano(), at)
	default:
		cursor = fmt.Sprintf("%d\n", base.Add(time.Duration(rng.Intn(600))*time.Second).UnixNano())
	}
	if rng.Intn(8) != 0 || cursor != "" {
		require.NoError(t, os.WriteFile(GetCursorFilePath(), []byte(cursor), 0o600))
	}

	masking := rng.Intn(2) == 0
	config := ShellTimeConfig{DataMasking: &masking}
	if rng.Intn(3) == 0 {
		config.Exclude = []string{"^export ", "[broken"}
	}
	return config
}

// BuildTrackingData reads a file store through a shortcut that leaves synced post
// commands undecoded. Whatever is in the files, it must return exactly what it returns
// when it decodes every one of them: the same data in the same order, the same cursor
// and meta, the same error.
func TestBuildTrackingData_FileStoreShortcutMatchesFullDecode(t *testing.T) {
	// The stores are full of lines no reader accepts, each logged at every read
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	ctx := context.Background()
	shortcutUsed, withData, withError := 0, 0, 0
	for seed := int64(1); seed <= 400; seed++ {
		t.Setenv("HOME", t.TempDir())
		config := randomStoreFiles(t, rand.New(rand.NewSource(seed)))
		store := newFileStore()

		want, wantErr := BuildTrackingData(ctx, plainStore{store}, config)
		got, gotErr := BuildTrackingData(ctx, store, config)

		if wantErr != nil {
			withError++
			require.Error(t, gotErr, "seed %d", seed)
			require.Equal(t, wantErr.Error(), gotErr.Error(), "seed %d", seed)
		} else {
			require.NoError(t, gotErr, "seed %d", seed)
		}
		require.Equal(t, want, got, "seed %d", seed)

		if len(want.Data) > 0 {
			withData++
		}
		if _, _, _, used, _ := pendingPostCommands(ctx, store); used {
			shortcutUsed++
		}
	}
	// The comparison is only worth something if the stores were varied
	t.Logf("400 stores: shortcut used on %d, %d with data to send, %d with an error", shortcutUsed, withData, withError)
	require.Greater(t, shortcutUsed, 100)
	require.Greater(t, withData, 100)
	require.Greater(t, withError, 10)
	require.Less(t, shortcutUsed, 400, "some stores must take the plain way too")
}

// The shortcut is taken by the file store itself, and only decodes what is waiting.
func TestFileStore_GetPostCommandsAfter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()
	var _ postsAfterCursorStore = store

	cursor := savePair(t, store, "git status", 0)
	savePair(t, store, "make build", time.Minute)
	// A synced line that doesn't decode is passed over like the other synced lines
	appendWithoutLock(t, GetPostCommandFilePath(), []byte(fmt.Sprintf("{broken\t%d\n", cursor.UnixNano())))

	posts, err := store.getPostCommandsAfter(ctx, cursor)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Equal(t, "make build", posts[0].Command)

	all, err := store.getPostCommandsAfter(ctx, time.Time{})
	require.NoError(t, err)
	require.Len(t, all, 2, "without a cursor every command that decodes is waiting")
}
