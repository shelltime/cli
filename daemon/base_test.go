package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitAndGetters(t *testing.T) {
	prevConfig := stConfig
	prevVersion := version
	prevStarted := startedAt
	t.Cleanup(func() {
		stConfig = prevConfig
		version = prevVersion
		startedAt = prevStarted
	})

	mockCS := model.NewMockConfigService(t)

	before := time.Now()
	Init(mockCS, "v9.9.9")
	after := time.Now()

	assert.Equal(t, "v9.9.9", GetVersion())

	started := GetStartedAt()
	assert.False(t, started.Before(before))
	assert.False(t, started.After(after))

	// stConfig was wired to the provided service.
	assert.Same(t, mockCS, stConfig)
}

func TestInitCommandStore(t *testing.T) {
	prev := commandStore
	t.Cleanup(func() { commandStore = prev })

	store := &fakeCommandStore{}
	InitCommandStore(store)
	require.NotNil(t, commandStore)
	assert.Same(t, store, commandStore)

	// Sanity: the registered store is usable.
	require.NoError(t, commandStore.SavePre(context.Background(), model.Command{Command: "x"}, time.Now()))
}
