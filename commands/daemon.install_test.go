package commands

import (
	"os"
	"path/filepath"
	"testing"
)

// TestShouldPreserveCurlDaemon pins the decision that gates renaming the
// curl-installer daemon to .bak. The critical case is the symlink alias: when
// resolution returns a path that is merely an alias of the curl binary, we must
// NOT move it aside — doing so was the cause of the Linux stuck loop where
// ~/.shelltime/bin ended up with shelltime-daemon.bak but no shelltime-daemon.
func TestShouldPreserveCurlDaemon(t *testing.T) {
	dir := t.TempDir()

	curlPath := filepath.Join(dir, "shelltime-daemon")
	if err := os.WriteFile(curlPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A genuinely different binary (e.g. a real Homebrew/system install).
	distinct := filepath.Join(dir, "system-shelltime-daemon")
	if err := os.WriteFile(distinct, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A symlink that points back at the curl binary — the same on-disk file.
	aliasDir := t.TempDir()
	alias := filepath.Join(aliasDir, "shelltime-daemon")
	if err := os.Symlink(curlPath, alias); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		daemonBinPath  string
		curlDaemonPath string
		want           bool
	}{
		{"identical path strings", curlPath, curlPath, false},
		{"genuinely distinct files", distinct, curlPath, true},
		{"resolved path is a symlink alias of curl", alias, curlPath, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldPreserveCurlDaemon(tc.daemonBinPath, tc.curlDaemonPath); got != tc.want {
				t.Errorf("shouldPreserveCurlDaemon(%q, %q) = %v, want %v",
					tc.daemonBinPath, tc.curlDaemonPath, got, tc.want)
			}
		})
	}
}
