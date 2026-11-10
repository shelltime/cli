package commands

import (
	"context"
	"log/slog"
	"os"

	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
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
	return model.CompactCommandFiles(ctx)
}

func commandGC(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "gc", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	storageFolder := model.GetBaseStoragePath()
	if _, err := os.Stat(storageFolder); os.IsNotExist(err) {
		return nil
	}

	// Get config for threshold
	cfg, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Warn("failed to read config, using default threshold", slog.Any("err", err))
		cfg.LogCleanup = &model.LogCleanup{ThresholdMB: 100}
	}
	thresholdBytes := cfg.LogCleanup.ThresholdMB * 1024 * 1024

	// Clean log files: force clean if --withLog, otherwise only clean large files
	forceCleanLogs := c.Bool("withLog")
	freedBytes, err := model.CleanLargeLogFiles(thresholdBytes, forceCleanLogs)
	if err != nil {
		slog.Warn("error during log cleanup", slog.Any("err", err))
	}
	if freedBytes > 0 {
		slog.Info("freed space from log files", slog.Int64("bytes", freedBytes))
	}

	if !c.Bool("skipLogCreation") {
		// only can setup logger after the log file clean
		SetupLogger(storageFolder)
		defer CloseLogger()
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
