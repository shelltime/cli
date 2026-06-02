package commands

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var TrackCommand *cli.Command = &cli.Command{
	Name:  "track",
	Usage: "track user commands",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "shell",
			Aliases: []string{"s"},
			Value:   "",
			Usage:   "the shell that user use",
		},
		&cli.Int64Flag{
			Name:    "sessionId",
			Aliases: []string{"id"},
			Value:   0,
			Usage:   "unix timestamp of the session",
		},
		&cli.StringFlag{
			Name:    "command",
			Aliases: []string{"cmd"},
			Value:   "",
			Usage:   "command that user executed",
		},
		&cli.StringFlag{
			Name:    "phase",
			Aliases: []string{"p"},
			Usage:   "Phase: pre, post",
		},
		&cli.IntFlag{
			Name:    "result",
			Aliases: []string{"r"},
			Usage:   "Exit code of last command",
		},
		&cli.IntFlag{
			Name:  "ppid",
			Value: 0,
			Usage: "Parent process ID of the shell (for terminal detection)",
		},
	},
	Action: commandTrack,
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		return nil
	},
}

func commandTrack(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "track", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	slog.Debug("track command args", slog.String("first", c.Args().First()))

	hostname, err := os.Hostname()
	if err != nil {
		slog.Error("failed to get hostname", slog.Any("err", err))
		return err
	}

	username := os.Getenv("USER")

	shell := c.String("shell")
	sessionId := c.Int64("sessionId")
	cmdCommand := c.String("command")
	cmdPhase := c.String("phase")
	result := c.Int("result")
	ppid := c.Int("ppid")

	instance := &model.Command{
		Shell:     shell,
		SessionID: sessionId,
		Command:   cmdCommand,
		Hostname:  hostname,
		Username:  username,
		Time:      time.Now(),
		Phase:     model.CommandPhasePre,
		PPID:      ppid,
	}

	// Fast path: `track` runs inside the shell hook on every command, so it must
	// stay cheap. A fresh `shelltime track` process is spawned per command, which
	// means the in-memory config cache never helps and reading the config would add
	// two TOML file reads to every command. Instead, if a daemon is listening on the
	// default socket, hand it the raw event (fire-and-forget) and return. The daemon
	// is a long-lived process: it reads config once (cached) and owns the
	// storage-engine decision (bolt vs txt), exclude filtering, sync and pruning.
	if daemon.IsSocketReady(ctx, model.DefaultSocketPath) {
		return sendTrackEventToDaemon(ctx, span, model.DefaultSocketPath, cmdPhase, instance, result)
	}

	// Slow path: no daemon on the default socket. We can now afford to read config —
	// it may point at a custom socket path, and it carries the exclude rules and the
	// settings the direct (daemon-less) sync needs.
	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("failed to read config file", slog.Any("err", err))
		return err
	}

	// Check if command should be excluded
	if model.ShouldExcludeCommand(cmdCommand, config.Exclude) {
		slog.Debug("Command excluded by pattern", slog.String("command", cmdCommand))
		return nil
	}

	// A daemon may still be listening on a non-default socket path.
	if config.SocketPath != model.DefaultSocketPath && daemon.IsSocketReady(ctx, config.SocketPath) {
		return sendTrackEventToDaemon(ctx, span, config.SocketPath, cmdPhase, instance, result)
	}

	// No daemon at all: persist to the local txt store and sync directly over HTTP.
	if cmdPhase == "pre" {
		span.SetAttributes(attribute.Int("phase", 0))
		err = instance.DoSavePre()
	}
	if cmdPhase == "post" {
		span.SetAttributes(attribute.Int("phase", 1))
		err = instance.DoUpdate(result)
	}
	if err != nil {
		slog.Error("failed to save/update command", slog.Any("err", err))
		return err
	}

	if cmdPhase == "post" {
		return trySyncLocalToServer(ctx, config, syncOptions{
			isDryRun:    false,
			isForceSync: false,
		})
	}
	return nil
}

// sendTrackEventToDaemon forwards a single raw pre/post command event to the
// daemon. The daemon owns persistence, exclude filtering, sync and pruning.
func sendTrackEventToDaemon(ctx context.Context, span trace.Span, socketPath, cmdPhase string, instance *model.Command, result int) error {
	now := time.Now()
	switch cmdPhase {
	case "pre":
		span.SetAttributes(attribute.Int("phase", 0))
		return daemon.SendTrackEvent(ctx, socketPath, daemon.SocketMessageTypeTrackPre, *instance, now)
	case "post":
		span.SetAttributes(attribute.Int("phase", 1))
		instance.Result = result
		return daemon.SendTrackEvent(ctx, socketPath, daemon.SocketMessageTypeTrackPost, *instance, now)
	}
	return nil
}

type syncOptions struct {
	isForceSync bool
	isDryRun    bool
}

func trySyncLocalToServer(
	ctx context.Context,
	config model.ShellTimeConfig,
	options syncOptions,
) error {
	isForceSync := options.isForceSync
	isDryRun := options.isDryRun

	// The local sync path always uses the txt file store. It must never open the
	// bolt DB, which the daemon owns exclusively.
	store := model.NewFileStore()

	result, err := model.BuildTrackingData(ctx, store, config)
	if err != nil {
		return err
	}

	if len(result.Data) == 0 {
		slog.Debug("no tracking data need to be sync")
		return nil
	}

	// no matter the flush count is, just force sync
	if !isForceSync {
		// allow first command to be sync with server
		if len(result.Data) < config.FlushCount && !result.NoCursorExist {
			slog.Debug("not enough data need to flush, abort", slog.Int("current", len(result.Data)))
			return nil
		}
	}

	err = DoSyncData(ctx, config, result.LatestRecordingTime, result.Data, result.Meta)
	if err != nil {
		slog.Error("Failed to send data to server", slog.Any("err", err))
		return err
	}

	if isDryRun {
		return nil
	}
	return store.SetCursor(ctx, result.LatestRecordingTime)
}

func DoSyncData(
	ctx context.Context,
	config model.ShellTimeConfig,
	cursor time.Time,
	trackingData []model.TrackingData,
	meta model.TrackingMetaData,
) error {
	socketPath := config.SocketPath
	isSocketReady := daemon.IsSocketReady(ctx, socketPath)

	slog.Debug("is socket ready", slog.Bool("ready", isSocketReady))

	// if the socket not ready, just call http to sync data
	if !isSocketReady {
		return model.SendLocalDataToServer(ctx, config, model.PostTrackArgs{
			CursorID: cursor.UnixNano(),
			Data:     trackingData,
			Meta:     meta,
		})
	}

	// send to socket if the socket is ready
	return daemon.SendLocalDataToSocket(ctx, socketPath, config, cursor, trackingData, meta)
}
