package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// fakeCommandStore is an in-memory CommandStore for handler tests.
type fakeCommandStore struct {
	pre  []*model.Command
	post []*model.Command

	cursor        time.Time
	noCursorExist bool

	cursorSetCalls int
	pruneCalls     int
}

func (f *fakeCommandStore) SavePre(ctx context.Context, cmd model.Command, rt time.Time) error {
	c := cmd
	c.RecordingTime = rt
	f.pre = append(f.pre, &c)
	return nil
}

func (f *fakeCommandStore) SavePost(ctx context.Context, cmd model.Command, result int, rt time.Time) error {
	c := cmd
	c.Result = result
	c.RecordingTime = rt
	f.post = append(f.post, &c)
	return nil
}

func (f *fakeCommandStore) GetPreTree(ctx context.Context) (map[string][]*model.Command, error) {
	tree := make(map[string][]*model.Command)
	for _, c := range f.pre {
		k := c.GetUniqueKey()
		tree[k] = append(tree[k], c)
	}
	return tree, nil
}

func (f *fakeCommandStore) GetPreCommands(ctx context.Context) ([]*model.Command, error) {
	return f.pre, nil
}

func (f *fakeCommandStore) GetPostCommands(ctx context.Context) ([]*model.Command, error) {
	return f.post, nil
}

func (f *fakeCommandStore) GetLastCursor(ctx context.Context) (time.Time, bool, error) {
	return f.cursor, f.noCursorExist, nil
}

func (f *fakeCommandStore) SetCursor(ctx context.Context, cursor time.Time) error {
	f.cursor = cursor
	f.cursorSetCalls++
	return nil
}

func (f *fakeCommandStore) Prune(ctx context.Context, cursor time.Time) error {
	f.pruneCalls++
	return nil
}

func (f *fakeCommandStore) Close() error { return nil }

// fakeConfigService implements model.ConfigService without mockery.
type fakeConfigService struct {
	cfg model.ShellTimeConfig
}

func (f fakeConfigService) ReadConfigFile(ctx context.Context, opts ...model.ReadConfigOption) (model.ShellTimeConfig, error) {
	return f.cfg, nil
}

type TrackHandlerTestSuite struct {
	suite.Suite
	prevStore  model.CommandStore
	prevConfig model.ConfigService
}

func (s *TrackHandlerTestSuite) SetupTest() {
	s.prevStore = commandStore
	s.prevConfig = stConfig
}

func (s *TrackHandlerTestSuite) TearDownTest() {
	commandStore = s.prevStore
	stConfig = s.prevConfig
}

func (s *TrackHandlerTestSuite) TestParseTrackEvent() {
	now := time.Now()
	payload := TrackEventPayload{
		Command:           model.Command{Shell: "bash", Command: "ls"},
		RecordingTimeNano: now.UnixNano(),
	}
	cmd, rt, err := parseTrackEvent(payload)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "ls", cmd.Command)
	assert.Equal(s.T(), now.UnixNano(), rt.UnixNano())
}

func (s *TrackHandlerTestSuite) TestTrackPreNoStore() {
	commandStore = nil
	err := handlePubSubTrackPre(context.Background(), TrackEventPayload{})
	assert.ErrorIs(s.T(), err, errNoCommandStore)
}

func (s *TrackHandlerTestSuite) TestTrackPrePersists() {
	store := &fakeCommandStore{}
	commandStore = store
	now := time.Now()
	payload := TrackEventPayload{
		Command:           model.Command{Shell: "bash", SessionID: 1, Command: "ls", Username: "u"},
		RecordingTimeNano: now.UnixNano(),
	}
	err := handlePubSubTrackPre(context.Background(), payload)
	require.NoError(s.T(), err)
	require.Len(s.T(), store.pre, 1)
	assert.Equal(s.T(), "ls", store.pre[0].Command)
}

func (s *TrackHandlerTestSuite) TestTrackPostNotEnoughToFlush() {
	store := &fakeCommandStore{
		// cursor exists, so FlushCount gating applies
		noCursorExist: false,
		cursor:        time.Now().Add(-time.Hour),
	}
	commandStore = store
	stConfig = fakeConfigService{cfg: model.ShellTimeConfig{FlushCount: 10}}

	now := time.Now()
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "ls", Username: "u", Time: now}
	// seed a matching pre so BuildTrackingData yields one data row
	require.NoError(s.T(), store.SavePre(context.Background(), cmd, now))

	payload := TrackEventPayload{Command: cmd, RecordingTimeNano: now.UnixNano()}
	err := handlePubSubTrackPost(context.Background(), payload)
	require.NoError(s.T(), err)

	// one data row < FlushCount(10): must not send / advance cursor / prune
	assert.Len(s.T(), store.post, 1, "post should still be persisted")
	assert.Equal(s.T(), 0, store.cursorSetCalls, "cursor must not advance below flush threshold")
	assert.Equal(s.T(), 0, store.pruneCalls)
}

func (s *TrackHandlerTestSuite) TestTrackPostFlushSyncsAndPrunes() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	store := &fakeCommandStore{noCursorExist: true}
	commandStore = store
	stConfig = fakeConfigService{cfg: model.ShellTimeConfig{Token: "t", APIEndpoint: server.URL, FlushCount: 1}}

	now := time.Now()
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "ls", Username: "u", Time: now}
	require.NoError(s.T(), store.SavePre(context.Background(), cmd, now))

	payload := TrackEventPayload{Command: cmd, RecordingTimeNano: now.UnixNano()}
	require.NoError(s.T(), handlePubSubTrackPost(context.Background(), payload))

	// flush threshold met (no cursor yet) => sent, cursor advanced, pruned
	assert.Len(s.T(), store.post, 1)
	assert.Equal(s.T(), 1, store.cursorSetCalls)
	assert.Equal(s.T(), 1, store.pruneCalls)
}

func (s *TrackHandlerTestSuite) TestTrackPostNoStore() {
	commandStore = nil
	err := handlePubSubTrackPost(context.Background(), TrackEventPayload{})
	assert.ErrorIs(s.T(), err, errNoCommandStore)
}

func TestTrackHandlerSuite(t *testing.T) {
	suite.Run(t, new(TrackHandlerTestSuite))
}
