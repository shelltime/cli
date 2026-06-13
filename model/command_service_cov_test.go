package model

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// m3mkExec creates an executable file (0755) at path, creating parent dirs.
func m3mkExec(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
}

// TestCommandService_LookPath_NvmCurrentFallback drives the env-var search loop:
// with an empty PATH and NVM_DIR set, the binary is found at
// $NVM_DIR/current/bin/<name>. (The home-dir entries use user.Current().HomeDir
// which we can't redirect, so we exercise the env-var-derived paths instead.)
func TestCommandService_LookPath_NvmCurrentFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix search paths")
	}
	nvm := t.TempDir()
	t.Setenv("PATH", "")
	t.Setenv("NVM_DIR", nvm)
	t.Setenv("FNM_DIR", "")
	t.Setenv("SHELL", "/bin/sh")

	name := "m3nvm"
	want := filepath.Join(nvm, "current", "bin", name)
	m3mkExec(t, want)

	svc := NewCommandService()
	got, err := svc.LookPath(name)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestCommandService_LookPath_NvmGlobVersions covers the glob branch for the
// $NVM_DIR/versions/node/*/bin/<name> pattern.
func TestCommandService_LookPath_NvmGlobVersions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix glob search paths")
	}
	nvm := t.TempDir()
	t.Setenv("PATH", "")
	t.Setenv("NVM_DIR", nvm)
	t.Setenv("FNM_DIR", "")
	t.Setenv("SHELL", "/bin/sh")

	name := "m3nvmglob"
	want := filepath.Join(nvm, "versions", "node", "v18.0.0", "bin", name)
	m3mkExec(t, want)

	svc := NewCommandService()
	got, err := svc.LookPath(name)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestCommandService_LookPath_GlobFnmVersions covers the filepath.Glob branch:
// the binary lives under a versioned fnm directory matched by a wildcard.
func TestCommandService_LookPath_GlobFnmVersions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix glob search paths")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	t.Setenv("NVM_DIR", "")
	t.Setenv("SHELL", "/bin/sh")

	name := "m3fnm"
	// Matches ~/.local/share/fnm/node-versions/*/installation/bin/<name>
	fnmDir := filepath.Join(home, ".fnm", "node-versions")
	t.Setenv("FNM_DIR", filepath.Join(home, ".fnm"))
	want := filepath.Join(fnmDir, "v20.0.0", "installation", "bin", name)
	m3mkExec(t, want)

	svc := NewCommandService()
	got, err := svc.LookPath(name)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestCommandService_LookPath_NonExecutableSkipped ensures a file that exists but
// is not executable is skipped (the mode&0111==0 branch), ultimately erroring.
func TestCommandService_LookPath_NonExecutableSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix executable-bit semantics")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses the executable-bit check")
	}
	nvm := t.TempDir()
	t.Setenv("PATH", "")
	t.Setenv("NVM_DIR", nvm)
	t.Setenv("FNM_DIR", "")
	t.Setenv("SHELL", "/bin/sh")

	name := "m3noexec"
	p := filepath.Join(nvm, "current", "bin", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	// Exists but not executable (0644) -> must be skipped.
	require.NoError(t, os.WriteFile(p, []byte("data"), 0o644))

	svc := NewCommandService()
	_, err := svc.LookPath(name)
	require.Error(t, err, "non-executable candidate must be skipped and overall lookup fails")
}
