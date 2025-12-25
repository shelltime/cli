package commands

import (
	"context"
	"fmt"
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
	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("failed to read config file", slog.Any("err", err))
		return err
	}

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

	instance := &model.Command{
		Shell:     shell,
		SessionID: sessionId,
		Command:   cmdCommand,
		Hostname:  hostname,
		Username:  username,
		Time:      time.Now(),
		Phase:     model.CommandPhasePre,
	}

	// Check if command should be excluded
	if model.ShouldExcludeCommand(cmdCommand, config.Exclude) {
		slog.Debug("Command excluded by pattern", slog.String("command", cmdCommand))
		return nil
	}

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
	postFileContent, lineCount, err := model.GetPostCommands(ctx)
	if err != nil {
		return err
	}

	if len(postFileContent) == 0 || lineCount == 0 {
		slog.Debug("Not enough records to sync", slog.Int("lineCount", lineCount))
		return nil
	}

	cursor, noCursorExist, err := model.GetLastCursor(ctx)
	if err != nil {
		return err
	}

	preFileTree, err := model.GetPreCommandsTree(ctx)
	if err != nil {
		return err
	}

	sysInfo, err := model.GetOSAndVersion()
	if err != nil {
		slog.Warn("failed to get OS version", slog.Any("err", err))
		sysInfo = &model.SysInfo{
			Os:      "unknown",
			Version: "unknown",
		}
	}

	trackingData := make([]model.TrackingData, 0)
	var latestRecordingTime time.Time = cursor

	meta := model.TrackingMetaData{
		Hostname:  "",
		Username:  "",
		OS:        sysInfo.Os,
		OSVersion: sysInfo.Version,
		Shell:     "",
	}

	for _, line := range postFileContent {
		postCommand := new(model.Command)
		recordingTime, err := postCommand.FromLineBytes(line)
		if err != nil {
			slog.Error("Failed to parse post command", slog.Any("err", err), slog.String("line", string(line)))
			continue
		}

		if recordingTime.Before(cursor) {
			continue
		}
		if recordingTime.After(latestRecordingTime) {
			latestRecordingTime = recordingTime
		}

		key := postCommand.GetUniqueKey()
		preCommands, ok := preFileTree[key]
		if !ok {
			continue
		}

		if meta.Hostname == "" {
			meta.Hostname = postCommand.Hostname
		}
		if meta.Shell == "" {
			meta.Shell = postCommand.Shell
		}
		if meta.Username == "" {
			meta.Username = postCommand.Username
		}

		// Check if command should be excluded during sync
		if model.ShouldExcludeCommand(postCommand.Command, config.Exclude) {
			slog.Debug("Command excluded during sync", slog.String("command", postCommand.Command))
			continue
		}

		// here very sure the commandList are all elligable, so no need check here.
		closestPreCommand := postCommand.FindClosestCommand(preCommands, false)

		td := model.TrackingData{
			SessionID:   postCommand.SessionID,
			Command:     postCommand.Command,
			EndTime:     postCommand.Time.Unix(),
			EndTimeNano: postCommand.Time.UnixNano(),
			Result:      postCommand.Result,
		}

		// data masking
		if config.DataMasking != nil && *config.DataMasking == true {
			td.Command = model.MaskSensitiveTokens(td.Command)
		}

		if closestPreCommand != nil {
			td.StartTime = closestPreCommand.Time.Unix()
			td.StartTimeNano = closestPreCommand.Time.UnixNano()
		}

		trackingData = append(trackingData, td)
	}

	if len(trackingData) == 0 {
		slog.Debug("no tracking data need to be sync")
		return nil
	}

	// no matter the flush count is, just force sync
	if !isForceSync {
		// allow first command to be sync with server
		if len(trackingData) < config.FlushCount && !noCursorExist {
			slog.Debug("not enough data need to flush, abort", slog.Int("current", len(trackingData)))
			return nil
		}
	}

	err = DoSyncData(ctx, config, latestRecordingTime, trackingData, meta)
	if err != nil {
		slog.Error("Failed to send data to server", slog.Any("err", err))
		return err
	}

	if isDryRun {
		return nil
	}
	// TODO: update cursor
	return updateCursorToFile(ctx, latestRecordingTime)
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

func updateCursorToFile(ctx context.Context, latestRecordingTime time.Time) error {
	ctx, span := commandTracer.Start(ctx, "updateCurosr")
	defer span.End()
	cursorFilePath := model.GetCursorFilePath()
	cursorFile, err := os.OpenFile(cursorFilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		slog.Error("Failed to open cursor file for writing", slog.Any("err", err))
		return err
	}
	defer cursorFile.Close()

	_, err = cursorFile.WriteString(fmt.Sprintf("\n%d\n", latestRecordingTime.UnixNano()))
	if err != nil {
		slog.Error("Failed to write to cursor file", slog.Any("err", err))
		return err
	}
	return nil
}
