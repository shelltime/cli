package model

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"
)

const (
	SEPARATOR = byte('\t')
)

var (
	COMMAND_BASE_STORAGE_FOLDER = ".shelltime"
	COMMAND_STORAGE_FOLDER      = COMMAND_BASE_STORAGE_FOLDER + "/commands"
	COMMAND_PRE_STORAGE_FILE    = COMMAND_STORAGE_FOLDER + "/pre.txt"
	COMMAND_POST_STORAGE_FILE   = COMMAND_STORAGE_FOLDER + "/post.txt"
	COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"
	HEARTBEAT_LOG_FILE          = COMMAND_BASE_STORAGE_FOLDER + "/coding-heartbeat.data.log"
	SYNC_PENDING_FILE           = COMMAND_BASE_STORAGE_FOLDER + "/sync-pending.jsonl"
)

func InitFolder(baseFolder string) {
	if baseFolder != "" {
		COMMAND_BASE_STORAGE_FOLDER = fmt.Sprintf(".shelltime-%s", baseFolder)
	}

	COMMAND_STORAGE_FOLDER = COMMAND_BASE_STORAGE_FOLDER + "/commands"
	COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"
	COMMAND_POST_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/post.txt"
	COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"
	HEARTBEAT_LOG_FILE = COMMAND_BASE_STORAGE_FOLDER + "/coding-heartbeat.data.log"
	SYNC_PENDING_FILE = COMMAND_BASE_STORAGE_FOLDER + "/sync-pending.jsonl"
}

// key: ${shell}|${sessionID}|${command}|${username}
// value: model.Command
type preCommandTree map[string][]*Command

func GetPreCommandsTree(ctx context.Context) (result preCommandTree, err error) {
	ctx, span := modelTracer.Start(ctx, "db.getPreCmdsTree")
	defer span.End()
	preFilePath := GetPreCommandFilePath()
	preFileHandler, err := os.Open(preFilePath)
	if err != nil {
		slog.Error("Failed to open pre-command file", slog.Any("err", err))
		return nil, err
	}
	defer preFileHandler.Close()

	result = make(preCommandTree)
	scanner := bufio.NewScanner(preFileHandler)
	buf := make([]byte, MAX_BUFFER_SIZE)
	scanner.Buffer(buf, MAX_BUFFER_SIZE)
	for scanner.Scan() {
		line := scanner.Bytes()
		cmd := new(Command)
		_, err := cmd.FromLineBytes(line)
		if err != nil {
			slog.Error("Invalid line parse in pre-command file", slog.String("line", string(line)), slog.Any("err", err))
			continue
		}

		key := cmd.GetUniqueKey()
		if len(result[key]) == 0 {
			result[key] = []*Command{cmd}
		} else {
			result[key] = append(result[key], cmd)
		}
	}

	if err := scanner.Err(); err != nil {
		return result, err
	}

	return result, nil
}

func GetPreCommands(ctx context.Context) ([]*Command, error) {
	ctx, span := modelTracer.Start(ctx, "db.getPreCmds")
	defer span.End()
	preFilePath := GetPreCommandFilePath()
	preFileHandler, err := os.Open(preFilePath)
	if err != nil {
		slog.Error("Failed to open pre-command file", slog.Any("err", err))
		return nil, err
	}
	defer preFileHandler.Close()

	result := make([]*Command, 0)
	scanner := bufio.NewScanner(preFileHandler)
	buf := make([]byte, MAX_BUFFER_SIZE)
	scanner.Buffer(buf, MAX_BUFFER_SIZE)

	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		cmd := new(Command)
		_, err := cmd.FromLineBytes(raw)
		if err != nil {
			slog.Error("Invalid line parse in pre-command file", slog.String("line", string(raw)), slog.Any("err", err))
			continue
		}
		result = append(result, cmd)
	}

	if err := scanner.Err(); err != nil {
		slog.Error("Error reading file", slog.Any("err", err))
		return nil, err
	}

	return result, nil
}

func GetLastCursor(ctx context.Context) (cursorTime time.Time, noCursorExist bool, err error) {
	ctx, span := modelTracer.Start(ctx, "db.getLastCursor")
	defer span.End()
	noCursorExist = false
	cursorFilePath := GetCursorFilePath()
	cursorFile, err := os.Open(cursorFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			cursorTime = time.Time{}
			noCursorExist = true
			err = nil
			return
		}
		slog.Error("Failed to open cursor file", slog.Any("err", err))
		return
	}
	defer cursorFile.Close()

	fileContent, err := io.ReadAll(cursorFile)

	if err != nil {
		slog.Error("Error reading cursor file", slog.Any("err", err))
		return cursorTime, false, err
	}

	var lastLine string
	for _, row := range bytes.Split(fileContent, []byte("\n")) {
		line := string(row)
		if line == "" {
			continue
		}
		lastLine = line
	}
	// if not data exists, just use time.Zero
	if lastLine == "" {
		return
	}

	cursor, err := strconv.Atoi(lastLine)
	if err != nil {
		slog.Error("Failed to parse cursor value", slog.Any("err", err))
		return
	}
	cursorTime = time.Unix(0, int64(cursor))
	return
}

func GetPostCommands(ctx context.Context) ([][]byte, int, error) {
	ctx, span := modelTracer.Start(ctx, "db.getPostCmds")
	defer span.End()
	postFilePath := GetPostCommandFilePath()
	postFileHandler, err := os.Open(postFilePath)
	if err != nil {
		slog.Error("Failed to open file", slog.Any("err", err))
		return nil, 0, err
	}
	defer postFileHandler.Close()

	scanner := bufio.NewScanner(postFileHandler)
	buf := make([]byte, MAX_BUFFER_SIZE)
	scanner.Buffer(buf, MAX_BUFFER_SIZE)
	nonEmptyContent := make([][]byte, 0)
	for scanner.Scan() {
		l := scanner.Bytes()
		if len(l) == 0 {
			continue
		}
		nonEmptyContent = append(nonEmptyContent, l)
	}

	if err := scanner.Err(); err != nil {
		slog.Error("Error reading file", slog.Any("err", err))
		return nil, 0, err
	}
	lineCount := len(nonEmptyContent)

	return nonEmptyContent, lineCount, nil
}
