package model

import (
	"context"
	"time"
)

// CommandStore abstracts persistence of tracked commands so the buffering
// backend can be swapped without touching call sites.
//
// Two implementations exist:
//   - fileStore: the historical append-only txt files (pre.txt/post.txt/cursor.txt).
//     Always available; used as the fallback whenever bolt is disabled or no daemon
//     is running.
//   - boltStore: a bbolt embedded KV store. bbolt holds an exclusive OS file lock,
//     so it is meant to be owned by the single long-lived daemon process.
//
// Pre commands live in the "active" bucket, post commands in "archived"; the
// sync cursor lives in a "meta" bucket. These names mirror the activeBucket /
// archivedBucket constants in command.go.
type CommandStore interface {
	// SavePre persists a pre-execution command record.
	SavePre(ctx context.Context, cmd Command, recordingTime time.Time) error
	// SavePost persists a post-execution command record with its exit code.
	SavePost(ctx context.Context, cmd Command, result int, recordingTime time.Time) error

	// GetPreTree returns pre commands indexed by their unique key, used to pair
	// post commands with the originating pre command during sync.
	GetPreTree(ctx context.Context) (map[string][]*Command, error)
	// GetPreCommands returns all pre commands (used by compaction).
	GetPreCommands(ctx context.Context) ([]*Command, error)
	// GetPostCommands returns all post commands with RecordingTime populated.
	GetPostCommands(ctx context.Context) ([]*Command, error)

	// GetLastCursor returns the last synced recording time. noCursorExist is true
	// when no cursor has ever been written (first sync).
	GetLastCursor(ctx context.Context) (cursorTime time.Time, noCursorExist bool, err error)
	// SetCursor advances the sync cursor.
	SetCursor(ctx context.Context, cursor time.Time) error

	// Prune removes records that have already been synced (recording time at or
	// before the cursor), keeping unfinished pre commands.
	Prune(ctx context.Context, cursor time.Time) error

	// Close releases any resources held by the store (no-op for fileStore).
	Close() error
}

// StorageEngineFile is the default, always-available txt-file backend.
const StorageEngineFile = "file"

// StorageEngineBolt selects the bbolt backend (daemon-owned).
const StorageEngineBolt = "bolt"

// NewCommandStore builds the store selected by config. It falls back to the
// file store for any unknown / empty engine so misconfiguration never loses data.
//
// The bolt store opens (and locks) the DB file; callers must Close it. Because
// bbolt takes an exclusive lock, only one process (the daemon) should construct
// a bolt store at a time.
func NewCommandStore(cfg ShellTimeConfig) (CommandStore, error) {
	engine := StorageEngineFile
	if cfg.Storage != nil && cfg.Storage.Engine != "" {
		engine = cfg.Storage.Engine
	}

	switch engine {
	case StorageEngineBolt:
		return newBoltStore(GetBoltDBPath())
	default:
		return newFileStore(), nil
	}
}
