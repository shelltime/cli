package model

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectVar_SetsCommitID(t *testing.T) {
	orig := commitID
	t.Cleanup(func() { commitID = orig })

	InjectVar("abc123")
	assert.Equal(t, "abc123", commitID)

	// updaterUserAgent reads commitID, giving an observable side-effect.
	assert.Equal(t, "shelltimeCLI@abc123", updaterUserAgent())
}

func TestSudoGetUserBaseFolder_NamedUser(t *testing.T) {
	got, err := SudoGetUserBaseFolder("alice")
	require.NoError(t, err)

	var prefix string
	switch runtime.GOOS {
	case "linux":
		prefix = "/home"
	case "darwin":
		prefix = "/Users"
	default:
		prefix = ""
	}
	assert.Equal(t, filepath.Join(prefix, "alice", ".shelltime"), got)
}

func TestSudoGetUserBaseFolder_EmptyUsernameErrorsWhenNoRoot(t *testing.T) {
	// With no username and (on most CI) no /root/.shelltime/bin, this errors.
	// On linux it may resolve to root if /root/.shelltime/bin exists; handle both.
	got, err := SudoGetUserBaseFolder("")
	if err != nil {
		assert.Contains(t, err.Error(), "could not find any user")
		assert.Empty(t, got)
		return
	}
	// If it did resolve, it must be the root path (linux-only branch).
	require.Equal(t, "linux", runtime.GOOS)
	assert.Equal(t, filepath.Join("/root", ".shelltime"), got)
}

func TestSudoGetBaseFolder_ContractHolds(t *testing.T) {
	// SudoGetBaseFolder scans the platform home prefix (/home or /Users) for a
	// user with a ~/.shelltime/bin directory. We can't seed that on the host,
	// so we assert the call's contract: on success the returned folder ends
	// with .shelltime and reflects the found user; on failure both are empty
	// with a descriptive error.
	folder, user, err := SudoGetBaseFolder()
	if err != nil {
		assert.Empty(t, folder)
		assert.Empty(t, user)
		assert.Contains(t, err.Error(), "could not find any user")
		return
	}
	assert.Contains(t, folder, ".shelltime")
	// folder must correspond to the found user (or the linux root special-case).
	if user != "" {
		assert.Contains(t, folder, user)
	}
}

func TestSudoGetUserBaseFolder_RootUserOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("root path special-cased on linux only")
	}
	got, err := SudoGetUserBaseFolder("root")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/root", ".shelltime"), got)
}
