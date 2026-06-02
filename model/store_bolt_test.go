package model

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type BoltStoreTestSuite struct {
	suite.Suite
	store *boltStore
	ctx   context.Context
}

func (s *BoltStoreTestSuite) SetupTest() {
	dbPath := filepath.Join(s.T().TempDir(), "commands.db")
	st, err := newBoltStore(dbPath)
	s.Require().NoError(err)
	s.store = st
	s.ctx = context.Background()
}

func (s *BoltStoreTestSuite) TearDownTest() {
	s.Require().NoError(s.store.Close())
}

func (s *BoltStoreTestSuite) newCmd(cmd string, t time.Time) Command {
	return Command{
		Shell:     "bash",
		SessionID: 1,
		Command:   cmd,
		Username:  "tester",
		Time:      t,
	}
}

func (s *BoltStoreTestSuite) TestSavePreAndRead() {
	now := time.Now()
	s.Require().NoError(s.store.SavePre(s.ctx, s.newCmd("ls", now), now))
	s.Require().NoError(s.store.SavePre(s.ctx, s.newCmd("pwd", now.Add(time.Second)), now.Add(time.Second)))

	pre, err := s.store.GetPreCommands(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(pre, 2)
	s.EqualValues(CommandPhasePre, pre[0].Phase)
	// RecordingTime recovered from the key
	s.Equal(now.UnixNano(), pre[0].RecordingTime.UnixNano())
}

func (s *BoltStoreTestSuite) TestTimeOrdering() {
	base := time.Now()
	// insert out of order
	s.Require().NoError(s.store.SavePre(s.ctx, s.newCmd("c", base.Add(2*time.Second)), base.Add(2*time.Second)))
	s.Require().NoError(s.store.SavePre(s.ctx, s.newCmd("a", base), base))
	s.Require().NoError(s.store.SavePre(s.ctx, s.newCmd("b", base.Add(time.Second)), base.Add(time.Second)))

	pre, err := s.store.GetPreCommands(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(pre, 3)
	s.Equal("a", pre[0].Command)
	s.Equal("b", pre[1].Command)
	s.Equal("c", pre[2].Command)
}

func (s *BoltStoreTestSuite) TestSameNanoNoCollision() {
	now := time.Now()
	s.Require().NoError(s.store.SavePre(s.ctx, s.newCmd("first", now), now))
	s.Require().NoError(s.store.SavePre(s.ctx, s.newCmd("second", now), now))

	pre, err := s.store.GetPreCommands(s.ctx)
	s.Require().NoError(err)
	s.Len(pre, 2, "identical recording times must not overwrite each other")
}

func (s *BoltStoreTestSuite) TestPreTree() {
	now := time.Now()
	c := s.newCmd("git status", now)
	s.Require().NoError(s.store.SavePre(s.ctx, c, now))

	tree, err := s.store.GetPreTree(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(tree[c.GetUniqueKey()], 1)
}

func (s *BoltStoreTestSuite) TestSavePost() {
	now := time.Now()
	s.Require().NoError(s.store.SavePost(s.ctx, s.newCmd("make", now), 2, now))

	post, err := s.store.GetPostCommands(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(post, 1)
	s.EqualValues(CommandPhasePost, post[0].Phase)
	s.Equal(2, post[0].Result)
}

func (s *BoltStoreTestSuite) TestCursor() {
	_, noCursor, err := s.store.GetLastCursor(s.ctx)
	s.Require().NoError(err)
	s.True(noCursor, "fresh db has no cursor")

	c := time.Unix(0, 1700000000123456789)
	s.Require().NoError(s.store.SetCursor(s.ctx, c))

	got, noCursor, err := s.store.GetLastCursor(s.ctx)
	s.Require().NoError(err)
	s.False(noCursor)
	s.Equal(c.UnixNano(), got.UnixNano())
}

func (s *BoltStoreTestSuite) TestPrune() {
	base := time.Now()
	synced := base
	unsyncedPost := base.Add(10 * time.Second)

	// a finished command (pre+post at the same time) before the cursor -> both pruned
	finished := s.newCmd("finished", synced)
	s.Require().NoError(s.store.SavePre(s.ctx, finished, synced))
	s.Require().NoError(s.store.SavePost(s.ctx, finished, 0, synced))

	// an unfinished pre before the cursor -> kept
	s.Require().NoError(s.store.SavePre(s.ctx, s.newCmd("running-server", synced), synced))

	// a post after the cursor -> kept
	s.Require().NoError(s.store.SavePost(s.ctx, s.newCmd("later", unsyncedPost), 0, unsyncedPost))

	s.Require().NoError(s.store.Prune(s.ctx, synced))

	pre, err := s.store.GetPreCommands(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(pre, 1)
	s.Equal("running-server", pre[0].Command)

	post, err := s.store.GetPostCommands(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(post, 1)
	s.Equal("later", post[0].Command)
}

func TestBoltStoreSuite(t *testing.T) {
	suite.Run(t, new(BoltStoreTestSuite))
}

func TestNewCommandStoreSelectsEngine(t *testing.T) {
	fileCfg := ShellTimeConfig{}
	st, err := NewCommandStore(fileCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := st.(*fileStore); !ok {
		t.Fatalf("expected fileStore by default, got %T", st)
	}
	_ = st.Close()
}
