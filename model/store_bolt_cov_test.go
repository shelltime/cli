package model

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// m3deleteBucket removes a named bucket from the store's DB so that subsequent
// operations hit the "bucket not found" guards. This is deterministic: we own
// the temp DB and explicitly drop the bucket.
func m3deleteBucket(t *testing.T, s *boltStore, name string) {
	t.Helper()
	require.NoError(t, s.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte(name))
	}))
}

func m3newBolt(t *testing.T) *boltStore {
	t.Helper()
	s, err := newBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestBoltStore_Put_MissingBucket covers the "bucket not found" guard inside put
// (reached via SavePre after the active bucket is dropped).
func TestBoltStore_Put_MissingBucket(t *testing.T) {
	s := m3newBolt(t)
	m3deleteBucket(t, s, activeBucket)

	err := s.SavePre(context.Background(), Command{Command: "x", Time: time.Now()}, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), activeBucket)
}

// TestBoltStore_All_MissingBucket covers the guard inside all() via
// GetPostCommands after dropping the archived bucket.
func TestBoltStore_All_MissingBucket(t *testing.T) {
	s := m3newBolt(t)
	m3deleteBucket(t, s, archivedBucket)

	_, err := s.GetPostCommands(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), archivedBucket)
}

// TestBoltStore_GetPreTree_MissingBucket covers GetPreTree's error propagation
// from all() when the active bucket is gone.
func TestBoltStore_GetPreTree_MissingBucket(t *testing.T) {
	s := m3newBolt(t)
	m3deleteBucket(t, s, activeBucket)

	_, err := s.GetPreTree(context.Background())
	require.Error(t, err)
}

// TestBoltStore_GetLastCursor_MissingMetaBucket covers the meta-bucket guard.
func TestBoltStore_GetLastCursor_MissingMetaBucket(t *testing.T) {
	s := m3newBolt(t)
	m3deleteBucket(t, s, metaBucket)

	_, _, err := s.GetLastCursor(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), metaBucket)
}

// TestBoltStore_SetCursor_MissingMetaBucket covers the meta-bucket guard in
// SetCursor.
func TestBoltStore_SetCursor_MissingMetaBucket(t *testing.T) {
	s := m3newBolt(t)
	m3deleteBucket(t, s, metaBucket)

	err := s.SetCursor(context.Background(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), metaBucket)
}

// TestBoltStore_Prune_MissingArchivedBucket covers Prune's archived-bucket guard.
func TestBoltStore_Prune_MissingArchivedBucket(t *testing.T) {
	s := m3newBolt(t)
	m3deleteBucket(t, s, archivedBucket)

	err := s.Prune(context.Background(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), archivedBucket)
}

// TestBoltStore_Prune_MissingActiveBucket covers Prune's active-bucket guard
// (archived present, active dropped).
func TestBoltStore_Prune_MissingActiveBucket(t *testing.T) {
	s := m3newBolt(t)
	m3deleteBucket(t, s, activeBucket)

	err := s.Prune(context.Background(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), activeBucket)
}
