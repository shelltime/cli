package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/term"
)

// setupHint is what gc prints at shell start while there is no config yet.
const setupHint = "ShellTime isn't set up yet. Run `shelltime init` to finish."

// Where gc prints the setup hint, and whether that is a terminal. Replaced in tests.
var (
	gcStderr           io.Writer = os.Stderr
	gcStderrIsTerminal           = func() bool { return term.IsTerminal(int(os.Stderr.Fd())) }
)

var GCCommand *cli.Command = &cli.Command{
	Name:  "gc",
	Usage: "clean internal storage",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "withLog",
			Aliases: []string{"wl"},
			Usage:   "clean the log file",
		},
		&cli.BoolFlag{
			Name:        "skipLogCreation",
			Aliases:     []string{"slc"},
			DefaultText: "false",
			Usage:       "skip log file creation",
		},
	},
	Action: commandGC,
}

// cleanCommandFiles compacts the txt command store: synced post commands and the pre
// commands they complete are dropped, and each file's previous content is kept as
// <file>.bak.
func cleanCommandFiles(ctx context.Context) error {
	commandsFolder := model.GetCommandsStoragePath()
	if _, err := os.Stat(commandsFolder); os.IsNotExist(err) {
		return nil
	}
	err := model.CompactCommandFiles(ctx)
	if errors.Is(err, model.ErrCommandStoreBusy) || errors.Is(err, model.ErrCommandStoreChanged) {
		// Another gc, or the daemon, is compacting right now, or rewrote a file under
		// this one. gc runs at every shell start: it must not hold the shell up or fail
		// it, and the next one compacts.
		slog.Debug("command store is in use, skipping compaction", slog.Any("reason", err))
		return nil
	}
	return err
}

func commandGC(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "gc", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	storageFolder := model.GetBaseStoragePath()
	if _, err := os.Stat(storageFolder); os.IsNotExist(err) {
		return nil
	}

	// Get config for threshold
	cfg, cfgErr := configService.ReadConfigFile(ctx)
	if cfgErr != nil {
		cfg.LogCleanup = &model.LogCleanup{ThresholdMB: 100}
	}
	notSetUp := errors.Is(cfgErr, model.ErrConfigNotFound)
	if notSetUp && gcStderrIsTerminal() {
		// Installed, but `init` hasn't run. The shell hooks run gc at every shell start,
		// so this is where the user hears what's left to do. Scripts (fish reads
		// config.fish for those too) get nothing.
		fmt.Fprintln(gcStderr, color.Yellow.Sprint(setupHint))
	}
	thresholdBytes := cfg.LogCleanup.ThresholdMB * 1024 * 1024

	// Clean log files: force clean if --withLog, otherwise only clean large files
	forceCleanLogs := c.Bool("withLog")
	freedBytes, cleanupErr := model.CleanLargeLogFiles(thresholdBytes, forceCleanLogs)

	if !c.Bool("skipLogCreation") {
		// only can setup logger after the log file clean
		SetupLogger(storageFolder)
		defer CloseLogger()
	}

	// Logged only now, so they go to the log file: gc runs at shell start, where
	// nothing but the setup hint belongs on the terminal.
	if cfgErr != nil && !notSetUp {
		slog.Warn("failed to read config, using default threshold", slog.Any("err", cfgErr))
	}
	if cleanupErr != nil {
		slog.Warn("error during log cleanup", slog.Any("err", cleanupErr))
	}
	if freedBytes > 0 {
		slog.Info("freed space from log files", slog.Int64("bytes", freedBytes))
	}

	// Clean command files. In bolt mode the daemon prunes synced commands from
	// the DB after each sync, and the txt files are only fallback leftovers, so
	// skip the txt compaction (post.txt may not even exist).
	if cfg.Storage == nil || cfg.Storage.Engine != model.StorageEngineBolt {
		if err := cleanCommandFiles(ctx); err != nil {
			return err
		}
	}

	// TODO: delete $HOME/.config/malamtime/ folder

	return nil
}
