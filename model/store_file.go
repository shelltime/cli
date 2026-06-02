package model

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"
)

// fileStore is the historical append-only txt backend. It delegates to the
// existing package-level reader functions so behavior is unchanged.
type fileStore struct{}

func newFileStore() *fileStore { return &fileStore{} }

func (s *fileStore) appendLine(path string, cmd Command, recordingTime time.Time) error {
	if err := ensureStorageFolder(); err != nil {
		return err
	}
	buf, err := cmd.ToLine(recordingTime)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open command storage file %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(buf); err != nil {
		return fmt.Errorf("failed to write command storage file %s: %w", path, err)
	}
	return nil
}

func (s *fileStore) SavePre(ctx context.Context, cmd Command, recordingTime time.Time) error {
	cmd.Phase = CommandPhasePre
	return s.appendLine(GetPreCommandFilePath(), cmd, recordingTime)
}

func (s *fileStore) SavePost(ctx context.Context, cmd Command, result int, recordingTime time.Time) error {
	cmd.Phase = CommandPhasePost
	cmd.Result = result
	cmd.EndTime = time.Now()
	return s.appendLine(GetPostCommandFilePath(), cmd, recordingTime)
}

func (s *fileStore) GetPreTree(ctx context.Context) (map[string][]*Command, error) {
	return GetPreCommandsTree(ctx)
}

func (s *fileStore) GetPreCommands(ctx context.Context) ([]*Command, error) {
	return GetPreCommands(ctx)
}

func (s *fileStore) GetPostCommands(ctx context.Context) ([]*Command, error) {
	raw, _, err := GetPostCommands(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*Command, 0, len(raw))
	for _, line := range raw {
		cmd := new(Command)
		if _, err := cmd.FromLineBytes(line); err != nil {
			slog.Warn("failed to parse post command line", slog.Any("err", err))
			continue
		}
		result = append(result, cmd)
	}
	return result, nil
}

func (s *fileStore) GetLastCursor(ctx context.Context) (time.Time, bool, error) {
	return GetLastCursor(ctx)
}

func (s *fileStore) SetCursor(ctx context.Context, cursor time.Time) error {
	if err := ensureStorageFolder(); err != nil {
		return err
	}
	cursorFile, err := os.OpenFile(GetCursorFilePath(), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer cursorFile.Close()
	_, err = cursorFile.WriteString(fmt.Sprintf("\n%d\n", cursor.UnixNano()))
	return err
}

// Prune compacts the txt files, dropping synced records and keeping unfinished
// pre commands. Mirrors the historical gc cleanCommandFiles behavior.
func (s *fileStore) Prune(ctx context.Context, cursor time.Time) error {
	postCommands, err := s.GetPostCommands(ctx)
	if err != nil {
		return err
	}
	if len(postCommands) == 0 {
		return nil
	}
	preCommands, err := s.GetPreCommands(ctx)
	if err != nil {
		return err
	}

	newPost := make([]*Command, 0)
	for _, cmd := range postCommands {
		if cmd != nil && cmd.RecordingTime.After(cursor) {
			newPost = append(newPost, cmd)
		}
	}

	newPre := make([]*Command, 0)
	for _, row := range preCommands {
		if row == nil {
			continue
		}
		if row.RecordingTime.After(cursor) {
			newPre = append(newPre, row)
			continue
		}
		// keep pre commands that never got a matching post (unfinished)
		closest := row.FindClosestCommand(postCommands, true)
		if closest == nil || closest.IsNil() {
			newPre = append(newPre, row)
		}
	}

	sort.Slice(newPre, func(i, j int) bool { return newPre[i].RecordingTime.Before(newPre[j].RecordingTime) })
	sort.Slice(newPost, func(i, j int) bool { return newPost[i].RecordingTime.Before(newPost[j].RecordingTime) })

	preBuf := bytes.Buffer{}
	for _, cmd := range newPre {
		line, err := cmd.ToLine(cmd.RecordingTime)
		if err != nil {
			return err
		}
		preBuf.Write(line)
	}
	postBuf := bytes.Buffer{}
	for _, cmd := range newPost {
		line, err := cmd.ToLine(cmd.RecordingTime)
		if err != nil {
			return err
		}
		postBuf.Write(line)
	}

	if err := os.WriteFile(GetPreCommandFilePath(), preBuf.Bytes(), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(GetPostCommandFilePath(), postBuf.Bytes(), 0644); err != nil {
		return err
	}
	return os.WriteFile(GetCursorFilePath(), []byte(fmt.Sprintf("%d", cursor.UnixNano())), 0644)
}

func (s *fileStore) Close() error { return nil }
