package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// x3TrackStore is a configurable in-memory CommandStore for exercising the error
// branches of the track handlers. Each *Err field, when set, makes the
// corresponding method return that error.
type x3TrackStore struct {
	pre  []*model.Command
	post []*model.Command

	cursor        time.Time
	noCursorExist bool

	savePreErr   error
	savePostErr  error
	getPostErr   error
	setCursorErr error
	pruneErr     error

	setCursorCalls int
	pruneCalls     int
}

func (s *x3TrackStore) SavePre(ctx context.Context, cmd model.Command, rt time.Time) error {
	if s.savePreErr != nil {
		return s.savePreErr
	}
	c := cmd
	c.RecordingTime = rt
	s.pre = append(s.pre, &c)
	return nil
}

func (s *x3TrackStore) SavePost(ctx context.Context, cmd model.Command, result int, rt time.Time) error {
	if s.savePostErr != nil {
		return s.savePostErr
	}
	c := cmd
	c.Result = result
	c.RecordingTime = rt
	s.post = append(s.post, &c)
	return nil
}

func (s *x3TrackStore) GetPreTree(ctx context.Context) (map[string][]*model.Command, error) {
	tree := make(map[string][]*model.Command)
	for _, c := range s.pre {
		k := c.GetUniqueKey()
		tree[k] = append(tree[k], c)
	}
	return tree, nil
}

func (s *x3TrackStore) GetPreCommands(ctx context.Context) ([]*model.Command, error) {
	return s.pre, nil
}

func (s *x3TrackStore) GetPostCommands(ctx context.Context) ([]*model.Command, error) {
	if s.getPostErr != nil {
		return nil, s.getPostErr
	}
	return s.post, nil
}

func (s *x3TrackStore) GetLastCursor(ctx context.Context) (time.Time, bool, error) {
	return s.cursor, s.noCursorExist, nil
}

func (s *x3TrackStore) SetCursor(ctx context.Context, cursor time.Time) error {
	s.setCursorCalls++
	if s.setCursorErr != nil {
		return s.setCursorErr
	}
	s.cursor = cursor
	return nil
}

func (s *x3TrackStore) Prune(ctx context.Context, cursor time.Time) error {
	s.pruneCalls++
	return s.pruneErr
}

func (s *x3TrackStore) Engine() string { return model.StorageEngineFile }

func (s *x3TrackStore) Close() error { return nil }

// x3SwapTrackGlobals saves and restores the daemon track globals around a test.
func x3SwapTrackGlobals(t *testing.T) {
	t.Helper()
	prevStore := commandStore
	prevConfig := stConfig
	prevFallback := newFallbackStore
	t.Cleanup(func() {
		commandStore = prevStore
		stConfig = prevConfig
		newFallbackStore = prevFallback
	})
}

func x3TrackPayload(cmd string) TrackEventPayload {
	now := time.Now()
	return TrackEventPayload{
		Command:           model.Command{Shell: "bash", SessionID: 1, Command: cmd, Username: "u", Hostname: "h", Time: now},
		RecordingTimeNano: now.UnixNano(),
	}
}

// TestX3TrackPre_ConfigReadError covers the config-read error branch of
// handlePubSubTrackPre.
func TestX3TrackPre_ConfigReadError(t *testing.T) {
	x3SwapTrackGlobals(t)
	commandStore = &x3TrackStore{}
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	stConfig = mc

	err := handlePubSubTrackPre(context.Background(), x3TrackPayload("ls"))
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

// TestX3TrackPre_SavePreError covers the SavePre error path of handlePubSubTrackPre.
func TestX3TrackPre_SavePreError(t *testing.T) {
	x3SwapTrackGlobals(t)
	commandStore = &x3TrackStore{savePreErr: errors.New("save pre boom")}
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
	stConfig = mc

	err := handlePubSubTrackPre(context.Background(), x3TrackPayload("ls"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save pre boom")
}

// TestX3TrackPost_ConfigReadError covers the config-read error branch of
// handlePubSubTrackPost.
func TestX3TrackPost_ConfigReadError(t *testing.T) {
	x3SwapTrackGlobals(t)
	commandStore = &x3TrackStore{}
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	stConfig = mc

	err := handlePubSubTrackPost(context.Background(), x3TrackPayload("ls"))
	require.Error(t, err)
}

// TestX3TrackPost_SavePostError covers the SavePost error path.
func TestX3TrackPost_SavePostError(t *testing.T) {
	x3SwapTrackGlobals(t)
	commandStore = &x3TrackStore{savePostErr: errors.New("save post boom")}
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
	stConfig = mc

	err := handlePubSubTrackPost(context.Background(), x3TrackPayload("ls"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save post boom")
}

// TestX3TrackPost_BuildTrackingDataError covers the BuildTrackingData error
// branch: GetPostCommands errors after the post is persisted.
func TestX3TrackPost_BuildTrackingDataError(t *testing.T) {
	x3SwapTrackGlobals(t)
	commandStore = &x3TrackStore{getPostErr: errors.New("get post boom")}
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)
	stConfig = mc

	err := handlePubSubTrackPost(context.Background(), x3TrackPayload("ls"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get post boom")
}

// TestX3TrackPost_SendErrorLeavesCursor covers the send-failure branch: the
// server returns 500, so sendTrackArgsToServer errors, the cursor is not
// advanced, and the data is left in the store.
func TestX3TrackPost_SendErrorLeavesCursor(t *testing.T) {
	x3SwapTrackGlobals(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	store := &x3TrackStore{noCursorExist: true}
	commandStore = store
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token: "t", APIEndpoint: srv.URL, FlushCount: 1,
	}, nil)
	stConfig = mc

	now := time.Now()
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "ls", Username: "u", Hostname: "h", Time: now}
	require.NoError(t, store.SavePre(context.Background(), cmd, now))

	err := handlePubSubTrackPost(context.Background(), TrackEventPayload{Command: cmd, RecordingTimeNano: now.UnixNano()})
	require.Error(t, err)
	assert.Equal(t, 0, store.setCursorCalls, "cursor must not advance on send failure")
}

// TestX3TrackPost_SetCursorError covers the SetCursor error branch after a
// successful send.
func TestX3TrackPost_SetCursorError(t *testing.T) {
	x3SwapTrackGlobals(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	store := &x3TrackStore{noCursorExist: true, setCursorErr: errors.New("cursor boom")}
	commandStore = store
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token: "t", APIEndpoint: srv.URL, FlushCount: 1,
	}, nil)
	stConfig = mc

	now := time.Now()
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "ls", Username: "u", Hostname: "h", Time: now}
	require.NoError(t, store.SavePre(context.Background(), cmd, now))

	err := handlePubSubTrackPost(context.Background(), TrackEventPayload{Command: cmd, RecordingTimeNano: now.UnixNano()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cursor boom")
}

// TestX3TrackPost_PruneErrorStillSucceeds covers the Prune warn-only branch: a
// Prune error after a successful send + cursor advance is logged but does not
// fail the handler.
func TestX3TrackPost_PruneErrorStillSucceeds(t *testing.T) {
	x3SwapTrackGlobals(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	store := &x3TrackStore{noCursorExist: true, pruneErr: errors.New("prune boom")}
	commandStore = store
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token: "t", APIEndpoint: srv.URL, FlushCount: 1,
	}, nil)
	stConfig = mc

	now := time.Now()
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "ls", Username: "u", Hostname: "h", Time: now}
	require.NoError(t, store.SavePre(context.Background(), cmd, now))

	err := handlePubSubTrackPost(context.Background(), TrackEventPayload{Command: cmd, RecordingTimeNano: now.UnixNano()})
	require.NoError(t, err, "Prune failure is warn-only and must not fail the handler")
	assert.Equal(t, 1, store.setCursorCalls)
	assert.Equal(t, 1, store.pruneCalls)
}

// --- codex usage sync small branches ------------------------------------------

// TestX3SendCodexUsageToServer_EmptyTokenNoop covers the empty-token early
// return of sendCodexUsageToServer (no HTTP request made).
func TestX3SendCodexUsageToServer_EmptyTokenNoop(t *testing.T) {
	err := sendCodexUsageToServer(context.Background(), model.ShellTimeConfig{Token: ""}, &CodexRateLimitData{})
	require.NoError(t, err)
}

// TestX3SyncCodexUsage_EmptyTokenNoop covers the empty-token early return of
// syncCodexUsage.
func TestX3SyncCodexUsage_EmptyTokenNoop(t *testing.T) {
	err := syncCodexUsage(context.Background(), model.ShellTimeConfig{Token: ""})
	require.NoError(t, err)
}
