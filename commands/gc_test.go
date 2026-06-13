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

// --- backupAndWriteFile (pure) ------------------------------------------------

func TestBackupAndWriteFile_NoExistingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.txt")
	require.NoError(t, backupAndWriteFile(p, []byte("hello")))

	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	// No backup created when the original did not exist.
	_, err = os.Stat(p + ".bak")
	assert.True(t, os.IsNotExist(err))
}

func TestBackupAndWriteFile_BacksUpExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(p, []byte("old"), 0644))

	require.NoError(t, backupAndWriteFile(p, []byte("new")))

	// New content written.
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))

	// Old content preserved in .bak.
	bak, err := os.ReadFile(p + ".bak")
	require.NoError(t, err)
	assert.Equal(t, "old", string(bak))
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
