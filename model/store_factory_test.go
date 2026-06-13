package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileStore_Engine(t *testing.T) {
	s := NewFileStore()
	require.NotNil(t, s)
	assert.Equal(t, StorageEngineFile, s.Engine())
	assert.NoError(t, s.Close())
}

func TestNewCommandStore_DefaultsToFile(t *testing.T) {
	// nil Storage -> file engine.
	s, err := NewCommandStore(ShellTimeConfig{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, StorageEngineFile, s.Engine())
	require.NoError(t, s.Close())

	// Unknown engine string falls back to file (never lose data).
	s2, err := NewCommandStore(ShellTimeConfig{Storage: &StorageConfig{Engine: "mystery"}})
	require.NoError(t, err)
	assert.Equal(t, StorageEngineFile, s2.Engine())
	require.NoError(t, s2.Close())
}

func TestNewCommandStore_BoltEngine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	InitFolder("") // point storage paths under the temp HOME
	// Bolt requires the commands dir to exist.
	require.NoError(t, ensureStorageFolder())

	s, err := NewCommandStore(ShellTimeConfig{Storage: &StorageConfig{Engine: StorageEngineBolt}})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, StorageEngineBolt, s.Engine())
	require.NoError(t, s.Close())
}

func TestPreHasSyncedPost(t *testing.T) {
	base := time.Now()
	pre := &Command{Shell: "bash", SessionID: 1, Command: "go test", Hostname: "h", Username: "u", Time: base}
	key := pre.GetUniqueKey()

	t.Run("nil pre returns false", func(t *testing.T) {
		assert.False(t, preHasSyncedPost(nil, nil, base))
	})

	t.Run("matching synced post returns true", func(t *testing.T) {
		post := &Command{Shell: "bash", SessionID: 1, Command: "go test", Hostname: "h", Username: "u",
			Time: base.Add(time.Second), RecordingTime: base.Add(time.Second)}
		require.Equal(t, key, post.GetUniqueKey())
		// cursor at/after post.RecordingTime => synced
		assert.True(t, preHasSyncedPost(pre, []*Command{post}, base.Add(2*time.Second)))
	})

	t.Run("post not yet synced (after cursor) returns false", func(t *testing.T) {
		post := &Command{Shell: "bash", SessionID: 1, Command: "go test", Hostname: "h", Username: "u",
			Time: base.Add(time.Second), RecordingTime: base.Add(10 * time.Second)}
		assert.False(t, preHasSyncedPost(pre, []*Command{post}, base.Add(2*time.Second)))
	})

	t.Run("different key returns false", func(t *testing.T) {
		post := &Command{Shell: "bash", SessionID: 2, Command: "other", Hostname: "h", Username: "u",
			Time: base.Add(time.Second), RecordingTime: base.Add(time.Second)}
		assert.False(t, preHasSyncedPost(pre, []*Command{post}, base.Add(2*time.Second)))
	})

	t.Run("post before pre returns false", func(t *testing.T) {
		// Same key but post.Time before pre.Time -> not a completion.
		post := &Command{Shell: "bash", SessionID: 1, Command: "go test", Hostname: "h", Username: "u",
			Time: base.Add(-time.Second), RecordingTime: base.Add(time.Second)}
		assert.False(t, preHasSyncedPost(pre, []*Command{post}, base.Add(2*time.Second)))
	})

	t.Run("nil entries in posts are skipped", func(t *testing.T) {
		assert.False(t, preHasSyncedPost(pre, []*Command{nil}, base.Add(2*time.Second)))
	})
}
