package commands

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// x3SetupTrack isolates HOME, installs a mock ConfigService, and skips the test
// if a real daemon happens to own the hardcoded default socket (which would
// otherwise divert commandTrack down the daemon path). Returns the temp HOME and
// the mock.
func x3SetupTrack(t *testing.T) (string, *model.MockConfigService) {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	if _, err := os.Stat(model.DefaultSocketPath); err == nil {
		t.Skip("default daemon socket present; commandTrack would take the daemon path")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USER", "tester")
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return home, mc
}

// x3UnreadySocket returns a socket path inside a temp dir that is guaranteed not
// to exist, so daemon.IsSocketReady reports false.
func x3UnreadySocket(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "absent.sock")
}

// TestX3CommandTrack_ConfigReadError covers the slow-path config read error in
// commandTrack (no default daemon socket present).
func TestX3CommandTrack_ConfigReadError(t *testing.T) {
	_, mc := x3SetupTrack(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)

	app := &cli.App{Name: "t", Commands: []*cli.Command{TrackCommand}}
	err := app.Run([]string{"t", "track", "--phase", "pre", "--shell", "bash", "--command", "ls", "--id", "1"})
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

// TestX3CommandTrack_ExcludedCommand covers the exclude-pattern branch: the
// command matches a config Exclude rule, so commandTrack returns nil without
// persisting anything.
func TestX3CommandTrack_ExcludedCommand(t *testing.T) {
	_, mc := x3SetupTrack(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		SocketPath: x3UnreadySocket(t),
		Exclude:    []string{"secret*"},
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{TrackCommand}}
	require.NoError(t, app.Run([]string{"t", "track", "--phase", "pre", "--shell", "bash", "--command", "secret-cmd", "--id", "1"}))

	// Excluded command must not create the pre storage file.
	_, statErr := os.Stat(filepath.Join(model.GetCommandsStoragePath(), "pre.txt"))
	assert.True(t, os.IsNotExist(statErr), "excluded command should not be persisted")
}

// TestX3CommandTrack_PrePersistsLocally covers the direct (daemon-less) pre
// branch: instance.DoSavePre writes to the local txt store.
func TestX3CommandTrack_PrePersistsLocally(t *testing.T) {
	_, mc := x3SetupTrack(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		SocketPath: x3UnreadySocket(t),
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{TrackCommand}}
	require.NoError(t, app.Run([]string{"t", "track", "--phase", "pre", "--shell", "bash", "--command", "echo hi", "--id", "7"}))

	data, err := os.ReadFile(filepath.Join(model.GetCommandsStoragePath(), "pre.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "echo hi")
}

// TestX3CommandTrack_PostSavesAndSyncs covers the post branch end-to-end:
// DoUpdate writes the post record, then trySyncLocalToServer -> DoSyncData sends
// the batch over HTTP (socket unready -> HTTP path). A matching pre is recorded
// first so BuildTrackingData yields a row, and FlushCount=1 forces the flush.
func TestX3CommandTrack_PostSavesAndSyncs(t *testing.T) {
	_, mc := x3SetupTrack(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	cfg := model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		SocketPath:  x3UnreadySocket(t),
		FlushCount:  1,
	}
	mc.On("ReadConfigFile", mock.Anything).Return(cfg, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{TrackCommand}}
	// Pre then post for the same session/command so a complete pair exists.
	require.NoError(t, app.Run([]string{"t", "track", "--phase", "pre", "--shell", "bash", "--command", "ls -la", "--id", "99"}))
	require.NoError(t, app.Run([]string{"t", "track", "--phase", "post", "--shell", "bash", "--command", "ls -la", "--id", "99", "--result", "0"}))

	assert.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(1), "post phase should sync the batch over HTTP")
}

// --- trySyncLocalToServer direct -----------------------------------------------

// x3SeedPair writes a completed pre/post pair into the local file store under
// the current HOME so BuildTrackingData produces exactly one data row.
func x3SeedPair(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	store := model.NewFileStore()
	now := time.Now().Add(-time.Minute)
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "git status", Username: "u", Hostname: "h", Time: now}
	require.NoError(t, store.SavePre(ctx, cmd, now))
	post := cmd
	post.Time = now.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))
}

// TestX3TrySyncLocalToServer_NoData covers the "no data to sync" early return.
func TestX3TrySyncLocalToServer_NoData(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	// The file store needs post.txt to exist; create an empty commands dir +
	// post.txt so BuildTrackingData yields zero rows (rather than an open error).
	cmdDir := model.GetCommandsStoragePath()
	require.NoError(t, os.MkdirAll(cmdDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "post.txt"), []byte(""), 0644))

	// Empty store -> BuildTrackingData yields zero rows -> nil.
	err := trySyncLocalToServer(context.Background(), model.ShellTimeConfig{}, syncOptions{})
	require.NoError(t, err)
}

// TestX3TrySyncLocalToServer_NotEnoughToFlush covers the FlushCount gating
// branch: one data row with a high FlushCount and an existing cursor aborts the
// sync without sending.
func TestX3TrySyncLocalToServer_NotEnoughToFlush(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	x3SeedPair(t)

	// Establish a cursor in the past so NoCursorExist is false and gating applies.
	store := model.NewFileStore()
	require.NoError(t, store.SetCursor(context.Background(), time.Now().Add(-2*time.Hour)))

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	cfg := model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		SocketPath:  filepath.Join(t.TempDir(), "absent.sock"),
		FlushCount:  100, // well above the single row available
	}
	require.NoError(t, trySyncLocalToServer(context.Background(), cfg, syncOptions{}))
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls), "should not flush below threshold")
}

// TestX3TrySyncLocalToServer_DryRunSkipsCursor covers the dry-run branch: data is
// sent but the cursor is not advanced.
func TestX3TrySyncLocalToServer_DryRunSkipsCursor(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	x3SeedPair(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	cfg := model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		SocketPath:  filepath.Join(t.TempDir(), "absent.sock"),
		FlushCount:  1,
	}
	require.NoError(t, trySyncLocalToServer(context.Background(), cfg, syncOptions{isDryRun: true, isForceSync: true}))
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "dry-run still sends the batch")

	// No cursor file should have been written by the dry-run.
	_, statErr := os.Stat(model.GetCursorFilePath())
	assert.True(t, os.IsNotExist(statErr), "dry-run must not advance the cursor")
}

// TestX3TrySyncLocalToServer_SyncErrorPropagates covers the DoSyncData error
// branch: the server returns 500, so the send fails and the error surfaces.
func TestX3TrySyncLocalToServer_SyncErrorPropagates(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	x3SeedPair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	cfg := model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		SocketPath:  filepath.Join(t.TempDir(), "absent.sock"),
		FlushCount:  1,
	}
	err := trySyncLocalToServer(context.Background(), cfg, syncOptions{isForceSync: true})
	require.Error(t, err)
}

// --- DoSyncData direct ---------------------------------------------------------

// TestX3DoSyncData_HTTPWhenSocketUnready covers the HTTP branch of DoSyncData
// when the configured socket is not ready.
func TestX3DoSyncData_HTTPWhenSocketUnready(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	cfg := model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		SocketPath:  filepath.Join(t.TempDir(), "absent.sock"),
	}
	data := []model.TrackingData{{Command: "ls", Result: 0}}
	meta := model.TrackingMetaData{OS: "linux", Shell: "bash"}
	require.NoError(t, DoSyncData(context.Background(), cfg, time.Now(), data, meta))
	assert.NotEmpty(t, gotPath, "HTTP sync endpoint should have been called")
}

// TestX3DoSyncData_SocketWhenReady covers the socket branch of DoSyncData: a
// ready unix socket makes it hand the batch to the daemon via SendLocalDataToSocket
// instead of HTTP. A minimal listener confirms a sync message arrives.
func TestX3DoSyncData_SocketWhenReady(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true

	socketPath := filepath.Join(t.TempDir(), "sync.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	got := make(chan daemon.SocketMessage, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		var msg daemon.SocketMessage
		if derr := json.NewDecoder(conn).Decode(&msg); derr == nil {
			got <- msg
		}
	}()

	cfg := model.ShellTimeConfig{Token: "tok", SocketPath: socketPath}
	data := []model.TrackingData{{Command: "ls", Result: 0}}
	meta := model.TrackingMetaData{OS: "linux", Shell: "bash"}
	require.NoError(t, DoSyncData(context.Background(), cfg, time.Now(), data, meta))

	select {
	case msg := <-got:
		assert.Equal(t, daemon.SocketMessageTypeSync, msg.Type)
	case <-time.After(time.Second):
		t.Fatal("sync message not delivered to socket")
	}
}
