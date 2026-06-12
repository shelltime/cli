package model

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildTrackingData(t *testing.T) {
	store, err := newBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	start := time.Now()
	cmd := Command{Shell: "bash", SessionID: 1, Command: "deploy prod", Username: "u", Hostname: "h", Time: start, PPID: 42}
	require.NoError(t, store.SavePre(ctx, cmd, start))
	post := cmd
	post.Time = start.Add(2 * time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))

	res, err := BuildTrackingData(ctx, store, ShellTimeConfig{})
	require.NoError(t, err)
	require.True(t, res.NoCursorExist)
	require.Len(t, res.Data, 1)
	require.Equal(t, "deploy prod", res.Data[0].Command)
	require.Equal(t, start.Unix(), res.Data[0].StartTime)
	require.Equal(t, post.Time.Unix(), res.Data[0].EndTime)
	require.Equal(t, 42, res.Data[0].PPID)
	require.Equal(t, "h", res.Meta.Hostname)
	require.Equal(t, "bash", res.Meta.Shell)
	require.Equal(t, StorageEngineBolt, res.Meta.CliEngine)
}

func TestBuildTrackingDataFileEngine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	InitFolder("") // reset globals to the default .shelltime under the temp HOME

	store := NewFileStore()
	defer store.Close()

	ctx := context.Background()
	start := time.Now()
	cmd := Command{Shell: "zsh", SessionID: 3, Command: "ls -la", Username: "u", Hostname: "h", Time: start}
	require.NoError(t, store.SavePre(ctx, cmd, start))
	post := cmd
	post.Time = start.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))

	res, err := BuildTrackingData(ctx, store, ShellTimeConfig{})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	require.Equal(t, StorageEngineFile, res.Meta.CliEngine)
}

func TestBuildTrackingDataExcludes(t *testing.T) {
	store, err := newBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	now := time.Now()
	cmd := Command{Shell: "bash", SessionID: 1, Command: "secret-token-cmd", Username: "u", Time: now}
	require.NoError(t, store.SavePre(ctx, cmd, now))
	post := cmd
	post.Time = now.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))

	res, err := BuildTrackingData(ctx, store, ShellTimeConfig{Exclude: []string{"^secret"}})
	require.NoError(t, err)
	require.Empty(t, res.Data, "excluded command must not be synced")
}

func TestBuildTrackingDataEmpty(t *testing.T) {
	store, err := newBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	require.NoError(t, err)
	defer store.Close()

	res, err := BuildTrackingData(context.Background(), store, ShellTimeConfig{})
	require.NoError(t, err)
	require.Empty(t, res.Data)
}
