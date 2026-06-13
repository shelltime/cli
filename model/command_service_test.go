package model

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommandService_NotNil(t *testing.T) {
	svc := NewCommandService()
	require.NotNil(t, svc)
	var _ CommandService = svc
}

func TestCommandService_LookPath_FastPathViaPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix executable bit semantics")
	}
	// Put a fake executable on an isolated PATH; exec.LookPath should find it
	// via the fast path before any of the home-directory fallbacks run.
	binDir := t.TempDir()
	exe := filepath.Join(binDir, "fakebin")
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir)

	svc := NewCommandService()
	got, err := svc.LookPath("fakebin")
	require.NoError(t, err)
	assert.Equal(t, exe, got)
}

func TestCommandService_LookPath_SystemBinaryViaPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only system path")
	}
	// /bin/sh exists on all unix systems; with /bin on PATH it resolves.
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh not present")
	}
	t.Setenv("PATH", "/bin:/usr/bin")
	svc := NewCommandService()
	got, err := svc.LookPath("sh")
	require.NoError(t, err)
	assert.Contains(t, got, "sh")
}

func TestCommandService_LookPath_AbsentReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only branch")
	}
	// Empty PATH + no NVM/FNM dirs forces the full fallback chain, which still
	// fails to find a bogus binary and returns a descriptive error.
	t.Setenv("PATH", "")
	t.Setenv("NVM_DIR", "")
	t.Setenv("FNM_DIR", "")
	t.Setenv("SHELL", "/bin/sh")

	svc := NewCommandService()
	_, err := svc.LookPath("definitely-not-a-real-binary-zzz-987")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definitely-not-a-real-binary-zzz-987")
}
