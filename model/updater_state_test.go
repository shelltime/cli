package model

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateState_RoundTrip(t *testing.T) {
	withTempHome(t)

	now := time.Now().UTC().Truncate(time.Second)
	want := UpdateState{
		LastCheckAt:         now,
		LastKnownTag:        "v0.1.90",
		LastAppliedTag:      "v0.1.90",
		PendingDaemonTag:    "v0.1.90",
		PendingDaemonPath:   "/tmp/shelltime-daemon.next",
		ConsecutiveFailures: 2,
	}
	require.NoError(t, WriteUpdateState(want))

	got, err := ReadUpdateState()
	require.NoError(t, err)
	assert.Equal(t, want.LastKnownTag, got.LastKnownTag)
	assert.Equal(t, want.LastAppliedTag, got.LastAppliedTag)
	assert.Equal(t, want.PendingDaemonTag, got.PendingDaemonTag)
	assert.Equal(t, want.PendingDaemonPath, got.PendingDaemonPath)
	assert.Equal(t, want.ConsecutiveFailures, got.ConsecutiveFailures)
	assert.True(t, want.LastCheckAt.Equal(got.LastCheckAt))
}

// State is a cache, never a source of truth: a missing or corrupt file must not
// stop the CLI from working.
func TestReadUpdateState_MissingFileIsZeroValue(t *testing.T) {
	withTempHome(t)

	got, err := ReadUpdateState()
	require.NoError(t, err)
	assert.Equal(t, UpdateState{}, got)
}

func TestReadUpdateState_CorruptFileIsZeroValue(t *testing.T) {
	withTempHome(t)
	require.NoError(t, WriteUpdateState(UpdateState{LastKnownTag: "v1"}))
	require.NoError(t, os.WriteFile(GetUpdateStatePath(), []byte("{not json"), 0o644))

	got, err := ReadUpdateState()
	require.NoError(t, err)
	assert.Equal(t, UpdateState{}, got)
}

func TestDaemonUpdatePendingMarker(t *testing.T) {
	withTempHome(t)

	_, err := ReadDaemonUpdatePending()
	assert.Error(t, err, "no marker yet")

	require.NoError(t, WriteDaemonUpdatePending("v0.1.90"))

	tag, err := ReadDaemonUpdatePending()
	require.NoError(t, err)
	assert.Equal(t, "v0.1.90", tag)

	require.NoError(t, ClearDaemonUpdatePending())
	_, err = ReadDaemonUpdatePending()
	assert.Error(t, err)

	// Clearing an already-absent marker is not an error.
	assert.NoError(t, ClearDaemonUpdatePending())
}
