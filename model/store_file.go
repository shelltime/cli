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
	"strconv"
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
	defer lockStoreForAppend()()

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

// getPostCommandsAfter returns the post commands recorded after cursor, in file order,
// as GetPostCommands followed by that filter would, without decoding the others. A line
// ends in its recording time, so the commands a sync already covered, which is most of
// the file between two compactions, are told apart by that number alone. BuildTrackingData
// runs for every post command and needs nothing else of them.
//
// any reports whether the store holds a post command at all, counting the ones at or
// before the cursor: with none, a build ends before it reads anything else. When nothing
// is waiting, that takes decoding synced lines until one of them is a command, usually
// the first.
func (s *fileStore) getPostCommandsAfter(ctx context.Context, cursor time.Time) (posts []*Command, any bool, err error) {
	raw, _, err := GetPostCommands(ctx)
	if err != nil {
		return nil, false, err
	}
	synced := func(line []byte) bool {
		recordingTime, ok := lineRecordingTime(line)
		return ok && !recordingTime.After(cursor)
	}
	posts = make([]*Command, 0)
	for _, line := range raw {
		if synced(line) {
			continue
		}
		// Everything else is decoded in full, also a line whose recording time can't be
		// read: it is refused there, with the same warning as before.
		cmd := new(Command)
		if _, err := cmd.FromLineBytes(line); err != nil {
			slog.Warn("failed to parse post command line", slog.Any("err", err))
			continue
		}
		posts = append(posts, cmd)
	}
	if len(posts) > 0 {
		return posts, true, nil
	}
	for _, line := range raw {
		if !synced(line) {
			continue // decoded above, and it was no command
		}
		if _, err := new(Command).FromLineBytes(line); err == nil {
			return posts, true, nil
		}
	}
	return posts, false, nil
}

// lineRecordingTime reads the recording time at the end of a store line, after its last
// tab, the way FromLineBytes does once it has decoded the command in front of it.
func lineRecordingTime(line []byte) (time.Time, bool) {
	tab := bytes.LastIndexByte(line, SEPARATOR)
	if tab < 0 {
		return time.Time{}, false
	}
	unixNano, err := strconv.ParseInt(string(line[tab+1:]), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(0, unixNano), true
}

func (s *fileStore) GetLastCursor(ctx context.Context) (time.Time, bool, error) {
	return GetLastCursor(ctx)
}

func (s *fileStore) SetCursor(ctx context.Context, cursor time.Time) error {
	if err := ensureStorageFolder(); err != nil {
		return err
	}
	defer lockStoreForAppend()()

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
	return s.compact(ctx, compactOptions{cursor: &cursor})
}

// CompactCommandFiles is `shelltime gc`'s compaction of the txt store: the same pruning
// the daemon does after each sync, up to the store's own cursor, keeping the previous
// content of each file as <file>.bak. gc runs it at every shell start, which is the only
// compaction a setup without a daemon gets.
//
// It returns ErrCommandStoreBusy, having changed nothing, when another process holds the
// store's lock for longer than commandStoreLockWait, and ErrCommandStoreChanged when it
// found a file rewritten under it.
func CompactCommandFiles(ctx context.Context) error {
	return newFileStore().compact(ctx, compactOptions{backup: true})
}

type compactOptions struct {
	// cursor is the time up to which commands are synced. Nil reads it from cursor.txt,
	// and then a store without a cursor keeps none: nothing was synced yet.
	cursor *time.Time
	// backup keeps the previous content of each rewritten file as <file>.bak
	backup bool
}

// compactAfterSnapshot, when set, runs between reading the files and replacing them.
// Tests use it to append at exactly the moment a rewrite used to lose a line.
var compactAfterSnapshot func()

// storeFileRewrite is one file of the store on its way through a compaction.
type storeFileRewrite struct {
	path string
	// snapshot is the content the compaction decided on
	snapshot []byte
	// kept is what stays of the snapshot
	kept []byte
}

// compact rewrites the txt files without the post commands at or before the cursor and
// the pre commands those complete.
//
// It holds the store's lock exclusively from reading the files to replacing them, so no
// append that takes the lock can fall in between. An append that doesn't take it (an older
// shelltime binary still running, or one that gave up waiting) is still carried over: the
// files are read again right before they are replaced, and whatever follows the content
// that was read first is kept as it is. A file that no longer starts with that content
// was rewritten by such a process, and the compaction stops with ErrCommandStoreChanged.
func (s *fileStore) compact(ctx context.Context, opts compactOptions) error {
	unlock, state := lockCommandStore(true)
	if state == storeLockBusy {
		return ErrCommandStoreBusy
	}
	defer unlock()

	postSnapshot, err := readStoreFile(GetPostCommandFilePath())
	if err != nil {
		return err
	}
	postCommands := parseCommandLines(postSnapshot)
	if len(postCommands) == 0 {
		return nil
	}
	preSnapshot, err := readStoreFile(GetPreCommandFilePath())
	if err != nil {
		return err
	}
	preCommands := parseCommandLines(preSnapshot)
	cursorSnapshot, err := readStoreFile(GetCursorFilePath())
	if err != nil {
		return err
	}

	var cursor time.Time
	writeCursor := true
	if opts.cursor != nil {
		cursor = *opts.cursor
	} else if cursor, writeCursor, err = parseLastCursor(cursorSnapshot); err != nil {
		slog.Error("Failed to parse cursor value", slog.Any("err", err))
		return err
	}

	newPost := make([]*Command, 0)
	for _, cmd := range postCommands {
		if cmd.RecordingTime.After(cursor) {
			newPost = append(newPost, cmd)
		}
	}

	newPre := make([]*Command, 0)
	for _, row := range preCommands {
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

	rewrites := []storeFileRewrite{
		{path: GetPreCommandFilePath(), snapshot: preSnapshot, kept: preBuf.Bytes()},
		{path: GetPostCommandFilePath(), snapshot: postSnapshot, kept: postBuf.Bytes()},
	}
	if writeCursor {
		rewrites = append(rewrites, storeFileRewrite{
			path: GetCursorFilePath(), snapshot: cursorSnapshot, kept: []byte(fmt.Sprintf("%d", cursor.UnixNano())),
		})
	}

	if compactAfterSnapshot != nil {
		compactAfterSnapshot()
	}

	// Before anything is replaced, check that every file still starts with what was read.
	// One that doesn't was rewritten by someone who doesn't take the lock (an older gc or
	// daemon), and what was decided above no longer describes the files.
	for _, rewrite := range rewrites {
		if _, err := rewrite.appendedSince(); err != nil {
			return err
		}
	}

	// The files are replaced one after the other, not together. A crash between two of
	// them, or a file found changed at the second look below, leaves the earlier ones
	// compacted and the later ones as they were: pre.txt without the pre commands of
	// posts that post.txt still holds, or both compacted next to the old cursor.txt.
	// Neither loses anything. Those posts are at or before the cursor, so no build looks
	// at them or needs their pre commands, the cursor a compaction writes is the one the
	// store already had or one a sync appended before calling it, and the next
	// compaction drops what this one left.
	write := writeFileAtomic
	if opts.backup {
		write = backupAndWriteFile
	}
	for _, rewrite := range rewrites {
		// Read again as late as possible: for an append made without the lock, the time
		// between this read and the rename is all that is left of the old race.
		appended, err := rewrite.appendedSince()
		if err != nil {
			return err
		}
		if err := write(rewrite.path, joinStoreContent(rewrite.kept, appended)); err != nil {
			return err
		}
	}
	return nil
}

// appendedSince returns what was appended to the file after its snapshot was read, or
// ErrCommandStoreChanged when the file no longer starts with the snapshot.
func (r storeFileRewrite) appendedSince() (appended []byte, err error) {
	current, err := readStoreFile(r.path)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(current, r.snapshot) {
		return nil, fmt.Errorf("%w: %s", ErrCommandStoreChanged, r.path)
	}
	return current[len(r.snapshot):], nil
}

// readStoreFile returns the content of one of the store's files, none for a missing one.
func readStoreFile(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return content, err
}

// parseCommandLines decodes the commands of a pre.txt or post.txt, skipping the lines
// that don't parse, as the readers do.
func parseCommandLines(content []byte) []*Command {
	commands := make([]*Command, 0)
	for _, line := range bytes.Split(content, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		cmd := new(Command)
		if _, err := cmd.FromLineBytes(line); err != nil {
			slog.Warn("failed to parse command line", slog.Any("err", err))
			continue
		}
		commands = append(commands, cmd)
	}
	return commands
}

// joinStoreContent puts what was appended to a file after its snapshot behind what a
// compaction kept of the snapshot, each on lines of its own.
func joinStoreContent(kept, appended []byte) []byte {
	if len(appended) == 0 {
		return kept
	}
	content := make([]byte, 0, len(kept)+1+len(appended))
	content = append(content, kept...)
	if len(kept) > 0 && kept[len(kept)-1] != '\n' && appended[0] != '\n' {
		content = append(content, '\n')
	}
	return append(content, appended...)
}

// writeFileAtomic replaces a file's content in one step: the new content is written next
// to it and renamed over it, so a reader gets the old file or the new one, never part of
// one, and a crash of the process leaves one of the two.
//
// The content is not synced to disk before the rename, like every other write of the
// store, so a power loss is not covered: the rename can survive it without the content.
// A sync here would run for three files at every shell start, where gc compacts.
func writeFileAtomic(filePath string, content []byte) error {
	tempFile := filePath + ".tmp"
	if err := WritePrivateFile(tempFile, content); err != nil {
		os.Remove(tempFile)
		slog.Warn("failed to write file", slog.String("file", filePath), slog.Any("err", err))
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}
	if err := os.Rename(tempFile, filePath); err != nil {
		os.Remove(tempFile)
		slog.Warn("failed to replace file", slog.String("file", filePath), slog.Any("err", err))
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}
	return nil
}

// backupAndWriteFile keeps the existing file's content as <file>.bak and replaces the file
// with new content. The backup is a copy: the file itself is never missing in between.
func backupAndWriteFile(filePath string, content []byte) error {
	previous, err := os.ReadFile(filePath)
	if err == nil {
		if err := WritePrivateFile(filePath+".bak", previous); err != nil {
			slog.Warn("failed to backup file", slog.String("file", filePath), slog.Any("err", err))
			return fmt.Errorf("failed to backup file %s: %w", filePath, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("failed to backup file", slog.String("file", filePath), slog.Any("err", err))
		return fmt.Errorf("failed to backup file %s: %w", filePath, err)
	}
	return writeFileAtomic(filePath, content)
}

func (s *fileStore) Engine() string { return StorageEngineFile }

func (s *fileStore) Close() error { return nil }
