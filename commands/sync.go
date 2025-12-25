package commands

import (
	"log/slog"
	"os"

	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
)

var SyncCommand *cli.Command = &cli.Command{
	Name:   "sync",
	Usage:  "manually sync local commands to server",
	Action: commandSync,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:        "dry-run",
			Aliases:     []string{"dr"},
			DefaultText: "false",
			Usage:       "Dry run only. do not do anything with side effect",
		},
	},
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		return nil
	},
}

func commandSync(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "sync", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("failed to read config file", slog.Any("err", err))
		return err
	}

	isDryRun := c.Bool("dry-run")
	return trySyncLocalToServer(ctx, config, syncOptions{
		isForceSync: true,
		isDryRun:    isDryRun,
	})
}
