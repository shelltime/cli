package model

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	// metaBucket holds singleton values such as the sync cursor.
	metaBucket = "meta"
	// cursorKey is the key under metaBucket storing the last synced recording time.
	cursorKey = "cursor"
	// boltOpenTimeout bounds how long we wait for the exclusive file lock before
	// giving up, so a stale lock can't hang the daemon forever.
	boltOpenTimeout = 5 * time.Second
)

// boltStore persists commands in a bbolt database. It holds an exclusive OS
// file lock for its lifetime, so only the daemon should own one.
type boltStore struct {
	db *bolt.DB
}

func newBoltStore(path string) (*boltStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create bolt db folder: %w", err)
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: boltOpenTimeout})
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt db %s: %w", path, err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{activeBucket, archivedBucket, metaBucket} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init bolt buckets: %w", err)
	}

	return &boltStore{db: db}, nil
}

// encodeKey produces a time-ordered, collision-free key:
// 8-byte big-endian UnixNano of the recording time + 8-byte sequence.
func encodeKey(recordingTime time.Time, seq uint64) []byte {
	key := make([]byte, 16)
	binary.BigEndian.PutUint64(key[0:8], uint64(recordingTime.UnixNano()))
	binary.BigEndian.PutUint64(key[8:16], seq)
	return key
}

func decodeKeyNano(key []byte) int64 {
	if len(key) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(key[0:8]))
}

func (s *boltStore) put(bucket string, cmd Command, recordingTime time.Time) error {
	val, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		return b.Put(encodeKey(recordingTime, seq), val)
	})
}

func (s *boltStore) all(bucket string) ([]*Command, error) {
	result := make([]*Command, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", bucket)
		}
		return b.ForEach(func(k, v []byte) error {
			cmd := new(Command)
			if err := json.Unmarshal(v, cmd); err != nil {
				slog.Warn("failed to unmarshal command from bolt", slog.Any("err", err))
				return nil
			}
			cmd.RecordingTime = time.Unix(0, decodeKeyNano(k))
			result = append(result, cmd)
			return nil
		})
	})
	return result, err
}

func (s *boltStore) SavePre(ctx context.Context, cmd Command, recordingTime time.Time) error {
	cmd.Phase = CommandPhasePre
	return s.put(activeBucket, cmd, recordingTime)
}

func (s *boltStore) SavePost(ctx context.Context, cmd Command, result int, recordingTime time.Time) error {
	cmd.Phase = CommandPhasePost
	cmd.Result = result
	cmd.EndTime = time.Now()
	return s.put(archivedBucket, cmd, recordingTime)
}

func (s *boltStore) GetPreTree(ctx context.Context) (map[string][]*Command, error) {
	cmds, err := s.all(activeBucket)
	if err != nil {
		return nil, err
	}
	tree := make(map[string][]*Command)
	for _, cmd := range cmds {
		key := cmd.GetUniqueKey()
		tree[key] = append(tree[key], cmd)
	}
	return tree, nil
}

func (s *boltStore) GetPreCommands(ctx context.Context) ([]*Command, error) {
	return s.all(activeBucket)
}

func (s *boltStore) GetPostCommands(ctx context.Context) ([]*Command, error) {
	return s.all(archivedBucket)
}

func (s *boltStore) GetLastCursor(ctx context.Context) (cursorTime time.Time, noCursorExist bool, err error) {
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metaBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", metaBucket)
		}
		v := b.Get([]byte(cursorKey))
		if v == nil {
			noCursorExist = true
			cursorTime = time.Time{}
			return nil
		}
		cursorTime = time.Unix(0, int64(binary.BigEndian.Uint64(v)))
		return nil
	})
	return
}

func (s *boltStore) SetCursor(ctx context.Context, cursor time.Time) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(cursor.UnixNano()))
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(metaBucket))
		if b == nil {
			return fmt.Errorf("bucket %s not found", metaBucket)
		}
		return b.Put([]byte(cursorKey), buf)
	})
}

// Prune deletes synced post commands (recording time <= cursor) and the pre
// commands they complete, keeping unfinished pre commands. It runs in a single
// write transaction so the post set is consistent with the deletions.
func (s *boltStore) Prune(ctx context.Context, cursor time.Time) error {
	cursorNano := cursor.UnixNano()

	return s.db.Update(func(tx *bolt.Tx) error {
		archived := tx.Bucket([]byte(archivedBucket))
		if archived == nil {
			return fmt.Errorf("bucket %s not found", archivedBucket)
		}
		active := tx.Bucket([]byte(activeBucket))
		if active == nil {
			return fmt.Errorf("bucket %s not found", activeBucket)
		}

		// Single pass over archived: collect all post commands (for matching) and
		// the synced keys to delete.
		var postCommands []*Command
		var delArchived [][]byte
		if err := archived.ForEach(func(k, v []byte) error {
			cmd := new(Command)
			if err := json.Unmarshal(v, cmd); err == nil {
				cmd.RecordingTime = time.Unix(0, decodeKeyNano(k))
				postCommands = append(postCommands, cmd)
			}
			if decodeKeyNano(k) <= cursorNano {
				delArchived = append(delArchived, append([]byte(nil), k...))
			}
			return nil
		}); err != nil {
			return err
		}

		// Drop pre rows at/before the cursor that a synced post completes.
		var delActive [][]byte
		if err := active.ForEach(func(k, v []byte) error {
			nano := decodeKeyNano(k)
			if nano > cursorNano {
				return nil // keep anything newer than the cursor
			}
			pre := new(Command)
			if err := json.Unmarshal(v, pre); err != nil {
				return nil
			}
			pre.RecordingTime = time.Unix(0, nano)
			if preHasSyncedPost(pre, postCommands, cursor) {
				delActive = append(delActive, append([]byte(nil), k...))
			}
			return nil
		}); err != nil {
			return err
		}

		for _, k := range delArchived {
			if err := archived.Delete(k); err != nil {
				return err
			}
		}
		for _, k := range delActive {
			if err := active.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *boltStore) Engine() string { return StorageEngineBolt }

func (s *boltStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
