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

// TestBuildTrackingData_MasksTokens covers the DataMasking branch: a JWT-shaped
// token in the command is masked in the resulting tracking payload.
func TestBuildTrackingData_MasksTokens(t *testing.T) {
	store, err := newBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	now := time.Now()
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJVadQssw5c"
	cmd := Command{Shell: "bash", SessionID: 1, Command: "curl -H 'auth: " + jwt + "'", Username: "u", Time: now}
	require.NoError(t, store.SavePre(ctx, cmd, now))
	post := cmd
	post.Time = now.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))

	masking := true
	res, err := BuildTrackingData(ctx, store, ShellTimeConfig{DataMasking: &masking})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	assert.NotContains(t, res.Data[0].Command, jwt, "JWT should be masked")
	assert.Contains(t, res.Data[0].Command, "***")
}

// TestBuildTrackingData_SkipsBeforeCursor covers the recordingTime.Before(cursor)
// continue branch: a post older than the cursor is excluded from the payload.
func TestBuildTrackingData_SkipsBeforeCursor(t *testing.T) {
	store, err := newBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	old := time.Now().Add(-time.Hour)
	recent := time.Now()

	// An old finished command, recorded before the cursor.
	oldCmd := Command{Shell: "bash", SessionID: 1, Command: "old", Username: "u", Time: old}
	require.NoError(t, store.SavePre(ctx, oldCmd, old))
	require.NoError(t, store.SavePost(ctx, oldCmd, 0, old))

	// A recent finished command, after the cursor.
	newCmd := Command{Shell: "bash", SessionID: 2, Command: "fresh", Username: "u", Time: recent}
	require.NoError(t, store.SavePre(ctx, newCmd, recent))
	require.NoError(t, store.SavePost(ctx, newCmd, 0, recent))

	// Cursor between the two: old is skipped, fresh is included.
	cursor := old.Add(30 * time.Minute)
	require.NoError(t, store.SetCursor(ctx, cursor))

	res, err := BuildTrackingData(ctx, store, ShellTimeConfig{})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "fresh", res.Data[0].Command)
}

// TestBuildTrackingData_SkipsPostWithoutPre covers the "no matching pre" continue
// branch: a post command whose pre was never recorded is not emitted.
func TestBuildTrackingData_SkipsPostWithoutPre(t *testing.T) {
	store, err := newBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	now := time.Now()
	// Only a post, no pre -> no preTree entry for its key.
	require.NoError(t, store.SavePost(ctx, Command{Shell: "bash", SessionID: 9, Command: "orphan", Username: "u", Time: now}, 0, now))

	res, err := BuildTrackingData(ctx, store, ShellTimeConfig{})
	require.NoError(t, err)
	assert.Empty(t, res.Data, "post without a paired pre is skipped")
}

// TestCollectWithIncludeSupport_DirectiveExpandErrorSkipped covers the branch
// where a directive's OriginalPath cannot be expanded (HOME unset + tilde), so
// it is dropped from the directive map and the path is treated as non-include.
func TestCollectWithIncludeSupport_DirectiveExpandErrorSkipped(t *testing.T) {
	t.Setenv("HOME", "")
	app := &BaseApp{name: "m3"}
	dir := t.TempDir()
	regular := filepath.Join(dir, "plain.conf")
	require.NoError(t, os.WriteFile(regular, []byte("hello\n"), 0o644))

	skip := true
	// Directive with a tilde OriginalPath: expandPath fails (no HOME) so it is not
	// registered; the plain file is still collected via the non-include path.
	directives := []IncludeDirective{{
		OriginalPath:  "~/cannot-expand",
		ShelltimePath: "~/cannot-expand.shelltime",
		IncludeLine:   "include",
		CheckString:   "cannot-expand",
	}}
	items, err := app.CollectWithIncludeSupport(context.Background(), "m3", []string{regular}, &skip, directives)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, regular, items[0].Path)
}

// TestCollectWithIncludeSupport_PathExpandErrorTreatedNonInclude covers the
// branch where the input path itself cannot be expanded: it falls into
// nonIncludePaths (and is then silently skipped by CollectFromPaths).
func TestCollectWithIncludeSupport_PathExpandErrorTreatedNonInclude(t *testing.T) {
	t.Setenv("HOME", "")
	app := &BaseApp{name: "m3"}
	skip := true
	items, err := app.CollectWithIncludeSupport(context.Background(), "m3", []string{"~/also-bad"}, &skip, nil)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// TestSaveWithIncludeSupport_NonShelltimeFallsToBaseSave covers the branch where
// a file path matches no directive and is diff-merged via the base Save.
func TestSaveWithIncludeSupport_NonShelltimeFallsToBaseSave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app := &BaseApp{name: "m3"}
	dir := t.TempDir()
	target := filepath.Join(dir, "plain.conf")

	// No directives -> target goes through base Save (new file write).
	require.NoError(t, app.SaveWithIncludeSupport(context.Background(),
		map[string]string{target: "content\n"}, false, nil))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "content\n", string(got))
}
