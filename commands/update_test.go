package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
)

// TestResolveDaemonDest pins that `shelltime update` writes the daemon next to
// the curl CLI even when another daemon is on PATH. Writing over a Homebrew
// cask symlink left an unmanaged binary that blocked later `brew install`s and
// kept the daemon service on an old release.
func TestResolveDaemonDest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	brewDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(brewDir, "shelltime-daemon"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", brewDir)

	want := model.GetCurlInstallerDaemonPath()
	if got := resolveDaemonDest(); got != want {
		t.Errorf("resolveDaemonDest() = %s, want %s", got, want)
	}
}
