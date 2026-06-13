package model

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOSAndVersion(t *testing.T) {
	info, err := GetOSAndVersion()
	// On supported platforms the helper commands exist; if they don't (minimal
	// containers without lsb_release/sw_vers), an error is acceptable and we
	// just assert the contract rather than a specific value.
	if err != nil {
		assert.Nil(t, info)
		return
	}
	require.NotNil(t, info)

	switch runtime.GOOS {
	case "linux":
		// Os/Version come from lsb_release; at minimum the call succeeded.
		// We don't pin a distro, but the struct should be populated when
		// lsb_release is present (it is in this environment).
		assert.NotEmpty(t, info.Os)
	case "darwin", "windows":
		assert.Equal(t, runtime.GOOS, info.Os)
		assert.NotEmpty(t, info.Version)
	default:
		assert.Equal(t, "unknown", info.Os)
		assert.Equal(t, "unknown", info.Version)
	}
}
