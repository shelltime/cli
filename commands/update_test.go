package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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

func TestCompareToLatest(t *testing.T) {
	tests := []struct {
		current, latest string
		want            int
	}{
		{"0.1.93", "v0.1.93", 0},
		{"0.1.90", "v0.1.93", -1},
		{"0.1.94", "v0.1.93", 1},
		{"0.1.94-next", "v0.1.93", 1},
		{"dev", "v0.1.93", -1},
	}
	for _, tt := range tests {
		if got := compareToLatest(tt.current, tt.latest); got != tt.want {
			t.Errorf("compareToLatest(%q, %q) = %d, want %d", tt.current, tt.latest, got, tt.want)
		}
	}
}

// TestRunDaemonReinstallUsesNewBinary pins that the daemon refresh runs the
// CLI that was just installed, not this (previous-release) process, so the
// service definition comes from the new release.
func TestRunDaemonReinstallUsesNewBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake CLI")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	cliPath := filepath.Join(dir, "shelltime")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n"
	if err := os.WriteFile(cliPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runDaemonReinstall(context.Background(), cliPath); err != nil {
		t.Fatalf("runDaemonReinstall() error = %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "daemon reinstall\n" {
		t.Errorf("new CLI ran with args %q, want %q", got, "daemon reinstall\n")
	}
}
