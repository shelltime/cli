package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestX3SetupLoggerAndClose drives the real SetupLogger/CloseLogger paths (with
// SKIP_LOGGER_SETTINGS temporarily disabled). It writes the log file under an
// isolated temp dir and restores logger state afterwards so other tests are
// unaffected.
func TestX3SetupLoggerAndClose(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())

	prevSkip := SKIP_LOGGER_SETTINGS
	SKIP_LOGGER_SETTINGS = false
	t.Cleanup(func() {
		CloseLogger()
		SKIP_LOGGER_SETTINGS = prevSkip
	})

	base := t.TempDir()
	// Fresh folder -> SetupLogger creates the dir + log.log, then opens it.
	SetupLogger(base)

	logPath := filepath.Join(base, "log.log")
	_, err := os.Stat(logPath)
	require.NoError(t, err, "SetupLogger should create the log file")

	// Second call with the file already present exercises the append-open path.
	SetupLogger(base)

	CloseLogger()
	// CloseLogger is idempotent (loggerFile is nil after the first close).
	assert.NotPanics(t, func() { CloseLogger() })
}

// TestX3CommandGC_CreatesLoggerWhenNotSkipped covers the !skipLogCreation branch
// of commandGC, which calls SetupLogger + defer CloseLogger. SKIP_LOGGER_SETTINGS
// is disabled for this test and restored afterwards.
func TestX3CommandGC_CreatesLoggerWhenNotSkipped(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())

	prevSkip := SKIP_LOGGER_SETTINGS
	SKIP_LOGGER_SETTINGS = false
	t.Cleanup(func() {
		CloseLogger()
		SKIP_LOGGER_SETTINGS = prevSkip
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	baseDir := model.GetBaseStoragePath()
	require.NoError(t, os.MkdirAll(baseDir, 0755))

	origCfg := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = origCfg })
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		LogCleanup: &model.LogCleanup{ThresholdMB: 100},
		Storage:    &model.StorageConfig{Engine: model.StorageEngineBolt}, // skip txt compaction
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GCCommand}}
	// No --skipLogCreation: commandGC sets up (and defers closing) the logger.
	require.NoError(t, app.Run([]string{"t", "gc"}))

	// The logger setup should have created log.log under the storage folder.
	_, statErr := os.Stat(filepath.Join(baseDir, "log.log"))
	assert.NoError(t, statErr, "gc without --skipLogCreation should create log.log")
}
