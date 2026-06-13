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

// c2SetupGC mirrors setupGCTest from gc_test.go but is independent so this file
// does not depend on helpers defined elsewhere. It isolates HOME to a temp dir
// and swaps in a mock ConfigService, restoring the original on cleanup.
func c2SetupGC(t *testing.T) (baseDir string, cmdDir string, mc *model.MockConfigService) {
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

// --- backupAndWriteFile failure branch ----------------------------------------

// TestBackupAndWriteFile_WriteIntoMissingDirFails hits the os.WriteFile error
// branch: the target's parent directory does not exist, so the write fails (and
// no prior file existed, so the rename/backup branch is skipped).
func TestC2BackupAndWriteFile_WriteIntoMissingDirFails(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	target := filepath.Join(missingDir, "data.txt")

	err := backupAndWriteFile(target, []byte("payload"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write file")
}

// --- cleanCommandFiles: pre without closest post is kept ----------------------

// TestC2CleanCommandFiles_KeepsUnmatchedPre exercises the branch where a pre
// command recorded before the cursor has no matching post; it must be retained
// in the rewritten pre.txt. It also drives a post-after-cursor (kept) so the
// post list is non-empty and the ToLine serialization for both files runs.
func TestC2CleanCommandFiles_KeepsUnmatchedPre(t *testing.T) {
	_, cmdDir, _ := c2SetupGC(t)
	require.NoError(t, os.MkdirAll(cmdDir, 0755))

	ctx := context.Background()
	store := model.NewFileStore()

	base := time.Now().Add(-2 * time.Hour)

	// An "orphan" pre recorded before the cursor with no matching post. It should
	// be preserved because FindClosestCommand returns nil.
	orphanPre := model.Command{Shell: "bash", SessionID: 11, Command: "long-running-server", Username: "u", Hostname: "h", Time: base}
	require.NoError(t, store.SavePre(ctx, orphanPre, base))

	// A completed pair recorded AFTER the cursor so the post survives compaction.
	livePre := model.Command{Shell: "bash", SessionID: 22, Command: "echo hi", Username: "u", Hostname: "h", Time: base.Add(90 * time.Minute)}
	require.NoError(t, store.SavePre(ctx, livePre, livePre.Time))
	livePost := livePre
	livePost.Time = livePre.Time.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, livePost, 0, livePost.Time))

	// Cursor sits between the orphan pre and the live pair.
	require.NoError(t, store.SetCursor(ctx, base.Add(30*time.Minute)))

	require.NoError(t, cleanCommandFiles(ctx))

	// The rewritten pre.txt must still contain the orphan command.
	preData, err := os.ReadFile(filepath.Join(cmdDir, "pre.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(preData), "long-running-server")

	// All three backups should have been created.
	for _, name := range []string{"pre.txt.bak", "post.txt.bak", "cursor.txt.bak"} {
		_, statErr := os.Stat(filepath.Join(cmdDir, name))
		assert.NoError(t, statErr, "expected backup %s", name)
	}
}

// --- commandGC: non-force large log file cleanup ------------------------------

// TestC2CommandGC_LargeLogFileCleanedWithoutForce covers the freedBytes>0 branch
// of commandGC without --withLog: a log file exceeding the configured threshold
// is removed by the size-based cleanup. A 1MB threshold keeps the test fast.
func TestC2CommandGC_LargeLogFileCleanedWithoutForce(t *testing.T) {
	baseDir, _, mc := c2SetupGC(t)
	require.NoError(t, os.MkdirAll(baseDir, 0755))

	// model.GetLogFilePath() == <base>/log.log; make it exceed a 1MB threshold.
	logPath := model.GetLogFilePath()
	require.NoError(t, os.WriteFile(logPath, make([]byte, 2*1024*1024), 0644))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		LogCleanup: &model.LogCleanup{ThresholdMB: 1},
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
	// No --withLog: cleanup is size-based; the oversized file is freed (freedBytes>0).
	require.NoError(t, app.Run([]string{"t", "gc", "--skipLogCreation"}))

	_, statErr := os.Stat(logPath)
	assert.True(t, os.IsNotExist(statErr), "oversized log.log should be removed by size-based cleanup")
}

// TestC2CommandGC_SmallLogFileKeptWithoutForce covers the opposite size branch:
// a sub-threshold log file is left in place when --withLog is not set.
func TestC2CommandGC_SmallLogFileKeptWithoutForce(t *testing.T) {
	baseDir, _, mc := c2SetupGC(t)
	require.NoError(t, os.MkdirAll(baseDir, 0755))

	logPath := model.GetLogFilePath()
	require.NoError(t, os.WriteFile(logPath, []byte("tiny\n"), 0644))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		LogCleanup: &model.LogCleanup{ThresholdMB: 100},
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
	require.NoError(t, app.Run([]string{"t", "gc", "--skipLogCreation"}))

	_, statErr := os.Stat(logPath)
	assert.NoError(t, statErr, "small log.log must be kept when below threshold and not forced")
}
