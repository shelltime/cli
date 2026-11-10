package model

import (
	"fmt"
	"os"
	"testing"
)

// TestMain gives the suite its own HOME. The storage paths are built from $HOME
// (GetBaseStoragePath), so a test that swaps COMMAND_BASE_STORAGE_FOLDER or
// SYNC_PENDING_FILE without also setting HOME writes into the real home
// directory, and a ShellTime daemon running on the machine (yours, or a Claude
// Code cloud session's) resends what it finds in ~/.shelltime to its account.
func TestMain(m *testing.M) {
	// A child of TestStoreLock_AppendsFromOtherProcessesSurviveCompactions: it works in
	// the HOME its parent gave it and runs no tests.
	if spec := os.Getenv(storeAppenderEnv); spec != "" {
		os.Exit(runStoreAppenderProcess(spec))
	}
	os.Exit(runInTempHome(m))
}

func runInTempHome(m *testing.M) int {
	home, err := os.MkdirTemp("", "shelltime-model-test-home-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create a HOME for the tests:", err)
		return 1
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	return m.Run()
}
