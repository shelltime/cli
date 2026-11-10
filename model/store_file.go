package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	f, err := openPrivateAppend(path)
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

// GetPreTree returns the pre commands by unique key. A store without pre.txt has none:
// the file is only created by the first pre command, and a post can get there first (the
// hooks were installed, or the daemon started, between a command's pre and its post).
// Failing on that made every batch build fail until some pre command arrived, and the
// daemon's track_post handler, nacked each time, appended the same post again on every
// redelivery.
func (s *fileStore) GetPreTree(ctx context.Context) (map[string][]*Command, error) {
	tree, err := GetPreCommandsTree(ctx)
	if errors.Is(err, fs.ErrNotExist) {
		return preCommandTree{}, nil
	}
	return tree, err
}

func (s *fileStore) GetPreCommands(ctx context.Context) ([]*Command, error) {
	commands, err := GetPreCommands(ctx)
	if errors.Is(err, fs.ErrNotExist) {
		return []*Command{}, nil
	}
	return commands, err
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
	cursorFile, err := openPrivateAppend(GetCursorFilePath())
	if err != nil {
		return err
	}
	defer cursorFile.Close()
	_, err = cursorFile.WriteString(fmt.Sprintf("\n%d\n", cursor.UnixNano()))
	return err
}

// Prune compacts the txt files, dropping synced records and keeping unfinished
// pre commands.
func (s *fileStore) Prune(ctx context.Context, cursor time.Time) error {
	return s.compact(ctx, cursor, compactOptions{writeCursor: true})
}

// CompactCommandFiles is `shelltime gc`'s compaction of the txt store: the same pruning
// the daemon does after each sync, up to the store's own cursor, keeping the previous
// content of each file as <file>.bak. gc runs it at every shell start, which is the only
// compaction a setup without a daemon gets.
func CompactCommandFiles(ctx context.Context) error {
	cursor, noCursorExist, err := GetLastCursor(ctx)
	if err != nil {
		return err
	}
	// Without a cursor nothing was synced yet: there is none to write back.
	return newFileStore().compact(ctx, cursor, compactOptions{backup: true, writeCursor: !noCursorExist})
}

type compactOptions struct {
	// backup keeps the previous content of each rewritten file as <file>.bak
	backup bool
	// writeCursor rewrites cursor.txt to hold just the cursor
	writeCursor bool
}

// compact rewrites the txt files without the post commands at or before cursor and the
// pre commands those complete.
func (s *fileStore) compact(ctx context.Context, cursor time.Time, opts compactOptions) error {
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
		// keep pre commands that no synced post completes (unfinished)
		if !preHasSyncedPost(row, postCommands, cursor) {
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

	write := WritePrivateFile
	if opts.backup {
		write = backupAndWriteFile
	}
	if err := write(GetPreCommandFilePath(), preBuf.Bytes()); err != nil {
		return err
	}
	if err := write(GetPostCommandFilePath(), postBuf.Bytes()); err != nil {
		return err
	}
	if !opts.writeCursor {
		return nil
	}
	return write(GetCursorFilePath(), []byte(fmt.Sprintf("%d", cursor.UnixNano())))
}

// backupAndWriteFile backs up the existing file as <file>.bak and writes new content.
func backupAndWriteFile(filePath string, content []byte) error {
	backupFile := filePath + ".bak"

	if _, err := os.Stat(filePath); err == nil {
		if err := os.Rename(filePath, backupFile); err != nil {
			slog.Warn("failed to backup file", slog.String("file", filePath), slog.Any("err", err))
			return fmt.Errorf("failed to backup file %s: %w", filePath, err)
		}
	}

	if err := WritePrivateFile(filePath, content); err != nil {
		slog.Warn("failed to write file", slog.String("file", filePath), slog.Any("err", err))
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

func (s *fileStore) Engine() string { return StorageEngineFile }

func (s *fileStore) Close() error { return nil }
