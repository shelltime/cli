package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// setupGCTest isolates HOME so the gc command operates entirely inside a
// throwaway temp dir. It intentionally does NOT touch model's package-level
// storage-folder globals (other suites depend on those); instead callers derive
// concrete paths from model.GetBaseStoragePath()/GetCommandsStoragePath(), which
// honor the temp HOME set here.
func setupGCTest(t *testing.T) (baseDir string, cmdDir string, mc *model.MockConfigService) {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true

	t.Setenv("HOME", t.TempDir())
	baseDir = model.GetBaseStoragePath()
	cmdDir = model.GetCommandsStoragePath()

	origConfig := configService
	mc = model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = origConfig })
	return baseDir, cmdDir, mc
}

// --- cleanCommandFiles --------------------------------------------------------

func TestCleanCommandFiles_NoCommandsFolder(t *testing.T) {
	setupGCTest(t)
	// commands folder absent -> returns nil immediately.
	require.NoError(t, cleanCommandFiles(context.Background()))
}

func TestCleanCommandFiles_NoPostCommands(t *testing.T) {
	_, cmdDir, _ := setupGCTest(t)
	// Create empty commands folder + empty post file -> postCount==0 -> nil.
	require.NoError(t, os.MkdirAll(cmdDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "post.txt"), []byte(""), 0644))

	require.NoError(t, cleanCommandFiles(context.Background()))
}

func TestCleanCommandFiles_CompactsSyncedCommands(t *testing.T) {
	_, cmdDir, _ := setupGCTest(t)
	require.NoError(t, os.MkdirAll(cmdDir, 0755))

	ctx := context.Background()
	store := model.NewFileStore()

	base := time.Now().Add(-time.Hour)
	// One completed pre/post pair, recorded before the cursor (=> already synced,
	// should be pruned).
	synced := model.Command{Shell: "bash", SessionID: 1, Command: "git status", Username: "u", Hostname: "h", Time: base}
	require.NoError(t, store.SavePre(ctx, synced, base))
	post := synced
	post.Time = base.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))

	// Set cursor to AFTER the post so it counts as synced.
	require.NoError(t, store.SetCursor(ctx, base.Add(2*time.Second)))

	require.NoError(t, cleanCommandFiles(ctx))

	// Backups of all three files should exist after compaction.
	for _, name := range []string{"pre.txt.bak", "post.txt.bak", "cursor.txt.bak"} {
		_, err := os.Stat(filepath.Join(cmdDir, name))
		assert.NoError(t, err, "expected backup %s", name)
	}
}

// gc is the only compaction a setup without a daemon gets. It used to match pre commands
// to posts the wrong way round (a post at or before the pre), so the pre of every finished,
// synced command stayed in pre.txt for good, and each later `track` read all of them.
func TestCleanCommandFiles_DropsPreOfSyncedPost(t *testing.T) {
	_, cmdDir, _ := setupGCTest(t)
	require.NoError(t, os.MkdirAll(cmdDir, 0755))

	ctx := context.Background()
	store := model.NewFileStore()
	base := time.Now().Add(-time.Hour)
	save := func(command string, startOffset time.Duration, finished bool) {
		pre := model.Command{Shell: "bash", SessionID: 1, Command: command, Username: "u", Hostname: "h", Time: base.Add(startOffset)}
		require.NoError(t, store.SavePre(ctx, pre, pre.Time))
		if finished {
			post := pre
			post.Time = pre.Time.Add(time.Second)
			require.NoError(t, store.SavePost(ctx, post, 0, post.Time))
		}
	}
	save("git status", 0, true)             // finished and synced
	save("npm run dev", time.Minute, false) // still running when the sync happened
	save("git status", 2*time.Minute, true) // same command again, finished and synced
	require.NoError(t, store.SetCursor(ctx, base.Add(10*time.Minute)))
	save("go test ./...", 20*time.Minute, true) // finished, not synced yet

	require.NoError(t, cleanCommandFiles(ctx))

	pres, err := store.GetPreCommands(ctx)
	require.NoError(t, err)
	var kept []string
	for _, pre := range pres {
		kept = append(kept, pre.Command)
	}
	assert.Equal(t, []string{"npm run dev", "go test ./..."}, kept, "only the unfinished and the unsynced pre stay")

	posts, err := store.GetPostCommands(ctx)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	assert.Equal(t, "go test ./...", posts[0].Command)

	// What is left still builds the one unsynced command, with its own start time
	res, err := model.BuildTrackingData(ctx, store, model.ShellTimeConfig{})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "go test ./...", res.Data[0].Command)
	assert.Equal(t, base.Add(20*time.Minute).Unix(), res.Data[0].StartTime)
}

// A store that never synced has no cursor, and gc must not invent one.
func TestCleanCommandFiles_NoCursorWritesNone(t *testing.T) {
	_, cmdDir, _ := setupGCTest(t)
	require.NoError(t, os.MkdirAll(cmdDir, 0755))

	ctx := context.Background()
	store := model.NewFileStore()
	at := time.Now().Add(-time.Minute)
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "ls", Username: "u", Hostname: "h", Time: at}
	require.NoError(t, store.SavePre(ctx, cmd, at))
	require.NoError(t, store.SavePost(ctx, cmd, 0, at.Add(time.Second)))

	require.NoError(t, cleanCommandFiles(ctx))

	_, noCursor, err := store.GetLastCursor(ctx)
	require.NoError(t, err)
	assert.True(t, noCursor, "the first sync is told apart by the missing cursor")
	posts, err := store.GetPostCommands(ctx)
	require.NoError(t, err)
	assert.Len(t, posts, 1, "nothing was synced, so nothing is dropped")
}

// --- commandGC action ---------------------------------------------------------

func TestCommandGC_NoStorageFolder(t *testing.T) {
	baseDir, _, _ := setupGCTest(t)
	// Ensure the storage folder really doesn't exist (temp HOME is empty anyway).
	require.NoError(t, os.RemoveAll(baseDir))

	app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
	// Early return before config is read; mock not set up on purpose.
	err := app.Run([]string{"t", "gc", "--skipLogCreation"})
	require.NoError(t, err)
}

func TestCommandGC_FileEngine_NoCommandData(t *testing.T) {
	baseDir, _, mc := setupGCTest(t)
	require.NoError(t, os.MkdirAll(baseDir, 0755))
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		LogCleanup: &model.LogCleanup{ThresholdMB: 100},
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
	// No commands folder -> cleanCommandFiles returns nil. skipLogCreation avoids
	// touching the logger global.
	err := app.Run([]string{"t", "gc", "--skipLogCreation"})
	require.NoError(t, err)
}

func TestCommandGC_BoltEngine_SkipsCommandCompaction(t *testing.T) {
	baseDir, _, mc := setupGCTest(t)
	require.NoError(t, os.MkdirAll(baseDir, 0755))
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		LogCleanup: &model.LogCleanup{ThresholdMB: 100},
		Storage:    &model.StorageConfig{Engine: model.StorageEngineBolt},
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
	err := app.Run([]string{"t", "gc", "--skipLogCreation"})
	require.NoError(t, err)
}

func TestCommandGC_ConfigReadErrorUsesDefaultThreshold(t *testing.T) {
	baseDir, _, mc := setupGCTest(t)
	require.NoError(t, os.MkdirAll(baseDir, 0755))
	// Config error -> default LogCleanup threshold applied; command still succeeds.
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
	err := app.Run([]string{"t", "gc", "--skipLogCreation"})
	require.NoError(t, err)
}

func TestCommandGC_WithLogForceCleansLogFile(t *testing.T) {
	baseDir, _, mc := setupGCTest(t)
	require.NoError(t, os.MkdirAll(baseDir, 0755))

	// Create a log file that --withLog should force-remove regardless of size.
	logPath := filepath.Join(baseDir, "log.log")
	require.NoError(t, os.WriteFile(logPath, []byte("some log content\n"), 0644))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		LogCleanup: &model.LogCleanup{ThresholdMB: 100},
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
	err := app.Run([]string{"t", "gc", "--withLog", "--skipLogCreation"})
	require.NoError(t, err)

	// The log file should have been removed by the forced cleanup.
	_, statErr := os.Stat(logPath)
	assert.True(t, os.IsNotExist(statErr), "log.log should be removed by --withLog")
}
