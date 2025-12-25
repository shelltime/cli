package commands

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"

	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
)

const logFileSizeThreshold int64 = 50 * 1024 * 1024 // 50 MB

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

// cleanLogFile removes a log file if it exceeds the threshold or if force is true.
// Returns the size of the deleted file (0 if not deleted or file doesn't exist).
func cleanLogFile(filePath string, threshold int64, force bool) (int64, error) {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	fileSize := info.Size()
	if !force && fileSize < threshold {
		return 0, nil
	}

	if err := os.Remove(filePath); err != nil {
		return 0, fmt.Errorf("failed to remove file %s: %w", filePath, err)
	}

	slog.Info("cleaned log file", slog.String("file", filePath), slog.Int64("size_bytes", fileSize))
	return fileSize, nil
}

// cleanLargeLogFiles checks all log files and removes those exceeding the size threshold.
// If force is true, removes all log files regardless of size.
func cleanLargeLogFiles(force bool) (int64, error) {
	logFiles := []string{
		model.GetLogFilePath(),
		model.GetHeartbeatLogFilePath(),
		model.GetSyncPendingFilePath(),
	}

	var totalFreed int64
	for _, filePath := range logFiles {
		freed, err := cleanLogFile(filePath, logFileSizeThreshold, force)
		if err != nil {
			slog.Warn("failed to clean log file", slog.String("file", filePath), slog.Any("err", err))
			continue
		}
		totalFreed += freed
	}

	return totalFreed, nil
}

// backupAndWriteFile backs up the existing file and writes new content.
func backupAndWriteFile(filePath string, content []byte) error {
	backupFile := filePath + ".bak"

	if _, err := os.Stat(filePath); err == nil {
		if err := os.Rename(filePath, backupFile); err != nil {
			slog.Warn("failed to backup file", slog.String("file", filePath), slog.Any("err", err))
			return fmt.Errorf("failed to backup file %s: %w", filePath, err)
		}
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		slog.Warn("failed to write file", slog.String("file", filePath), slog.Any("err", err))
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// cleanCommandFiles cleans up the command storage files based on the cursor position.
func cleanCommandFiles(ctx context.Context) error {
	commandsFolder := model.GetCommandsStoragePath()
	if _, err := os.Stat(commandsFolder); os.IsNotExist(err) {
		return nil
	}

	lastCursor, _, err := model.GetLastCursor(ctx)
	if err != nil {
		return err
	}

	postCommandsRaw, postCount, err := model.GetPostCommands(ctx)
	if err != nil {
		return err
	}

	postCommands := make([]*model.Command, len(postCommandsRaw))
	for i, raw := range postCommandsRaw {
		cmd := new(model.Command)
		_, err := cmd.FromLineBytes(raw)
		if err != nil {
			slog.Warn("failed to parse command from line", slog.Any("err", err))
			continue
		}
		postCommands[i] = cmd
	}

	if postCount == 0 {
		slog.Debug("no post commands need to be clean")
		return nil
	}

	preCommands, err := model.GetPreCommands(ctx)
	if err != nil {
		return err
	}

	newPreCommandList := make([]*model.Command, 0)
	newPostCommandList := make([]*model.Command, 0)

	// save all the data that before cursor
	for _, cmd := range postCommands {
		if cmd == nil {
			continue
		}
		if cmd.RecordingTime.After(lastCursor) {
			newPostCommandList = append(newPostCommandList, cmd)
		}
	}

	// If there is no end, it should be kept. For example, if one tab opened a webpack dev server and the user opened another tab, we should keep the previous pre
	for _, row := range preCommands {
		recordingTime := row.RecordingTime
		// If it's data after the cursor, save it without thinking
		if recordingTime.After(lastCursor) {
			newPreCommandList = append(newPreCommandList, row)
			continue
		}

		// if the closest node not found, prohaps the pre command not finished yet. save the pre command anyway
		closestNode := row.FindClosestCommand(postCommands, true)
		if closestNode == nil || closestNode.IsNil() {
			newPreCommandList = append(newPreCommandList, row)
		}
	}

	sort.Slice(newPreCommandList, func(i, j int) bool {
		return newPreCommandList[i].
			RecordingTime.
			Before(
				newPreCommandList[j].RecordingTime,
			)
	})

	sort.Slice(newPostCommandList, func(i, j int) bool {
		return newPostCommandList[i].
			RecordingTime.
			Before(
				newPostCommandList[j].RecordingTime,
			)
	})

	// Build pre file content
	preFileContent := bytes.Buffer{}
	for _, cmd := range newPreCommandList {
		line, err := cmd.ToLine(cmd.RecordingTime)
		if err != nil {
			return fmt.Errorf("failed to convert command to line: %w", err)
		}
		preFileContent.Write(line)
	}

	// Build post file content
	postFileContent := bytes.Buffer{}
	for _, cmd := range newPostCommandList {
		line, err := cmd.ToLine(cmd.RecordingTime)
		if err != nil {
			return fmt.Errorf("failed to convert command to line: %w", err)
		}
		postFileContent.Write(line)
	}

	// Build cursor file content
	lastCursorNano := lastCursor.UnixNano()
	cursorContent := []byte(fmt.Sprintf("%d", lastCursorNano))

	// Backup and write all files
	if err := backupAndWriteFile(model.GetPreCommandFilePath(), preFileContent.Bytes()); err != nil {
		return err
	}

	if err := backupAndWriteFile(model.GetPostCommandFilePath(), postFileContent.Bytes()); err != nil {
		return err
	}

	if err := backupAndWriteFile(model.GetCursorFilePath(), cursorContent); err != nil {
		return err
	}

	return nil
}

func commandGC(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "gc", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	storageFolder := model.GetBaseStoragePath()
	if _, err := os.Stat(storageFolder); os.IsNotExist(err) {
		return nil
	}

	// Clean log files: force clean if --withLog, otherwise only clean large files
	forceCleanLogs := c.Bool("withLog")
	freedBytes, err := cleanLargeLogFiles(forceCleanLogs)
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

	// Clean command files
	if err := cleanCommandFiles(ctx); err != nil {
		return err
	}

	// TODO: delete $HOME/.config/malamtime/ folder

	return nil
}
