package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DisableAutoUpdateEnv is an emergency kill switch for the whole self-update
// system. Setting it to any non-empty value stops the daemon from checking and
// stops the CLI from applying a staged update, without requiring a config edit.
const DisableAutoUpdateEnv = "SHELLTIME_DISABLE_AUTO_UPDATE"

// UpdateState is the durable handoff between the daemon (which checks and
// downloads) and the CLI (which finishes the daemon swap on a later shell).
//
// It is persisted rather than kept in memory because the two halves run in
// different processes, and because a daemon restart must not reset the check
// cadence — otherwise a daemon that restarts more often than the check interval
// would never check at all.
type UpdateState struct {
	// LastCheckAt gates the daily check across daemon restarts.
	LastCheckAt time.Time `json:"lastCheckAt"`
	// LastKnownTag is the newest tag the server has reported.
	LastKnownTag string `json:"lastKnownTag,omitempty"`
	// LastAppliedTag is the tag we last successfully downloaded and installed,
	// so a restart doesn't re-download the same release.
	LastAppliedTag string `json:"lastAppliedTag,omitempty"`
	// PendingDaemonTag/PendingDaemonPath describe a staged daemon binary waiting
	// for the CLI to activate it. PendingDaemonPath is empty for Homebrew
	// installs, where brew owns the binary and only a service restart is needed.
	PendingDaemonTag  string `json:"pendingDaemonTag,omitempty"`
	PendingDaemonPath string `json:"pendingDaemonPath,omitempty"`
	// Notice is a user-facing one-liner shown once per day at shell startup.
	Notice        string    `json:"notice,omitempty"`
	NoticeShownAt time.Time `json:"noticeShownAt,omitempty"`
	// LastDaemonStartAttemptAt rate-limits self-healing restarts of a daemon
	// that is down but cannot start.
	LastDaemonStartAttemptAt time.Time `json:"lastDaemonStartAttemptAt,omitempty"`
	// LastError and ConsecutiveFailures drive the exponential backoff.
	LastError           string `json:"lastError,omitempty"`
	ConsecutiveFailures int    `json:"consecutiveFailures,omitempty"`
}

// ReadUpdateState loads the persisted state. A missing or unreadable file yields
// the zero value with no error: update state is a cache, never a source of
// truth, and a corrupt file must not stop the CLI from working.
func ReadUpdateState() (UpdateState, error) {
	var st UpdateState
	raw, err := os.ReadFile(GetUpdateStatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, nil
		}
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return UpdateState{}, nil
	}
	return st, nil
}

// WriteUpdateState persists state atomically (temp file + rename), so a reader
// never observes a half-written file.
func WriteUpdateState(st UpdateState) error {
	path := GetUpdateStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// WriteDaemonUpdatePending drops the marker the CLI hot path looks for. It is
// written last, after the state file, so the CLI never sees a marker without
// the state that explains it.
func WriteDaemonUpdatePending(tag string) error {
	path := GetDaemonUpdatePendingPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(tag), 0o644)
}

// ReadDaemonUpdatePending returns the tag recorded in the pending marker.
func ReadDaemonUpdatePending() (string, error) {
	raw, err := os.ReadFile(GetDaemonUpdatePendingPath())
	if err != nil {
		return "", err
	}
	tag := string(raw)
	if tag == "" {
		return "", fmt.Errorf("empty pending marker")
	}
	return tag, nil
}

// ClearDaemonUpdatePending removes the marker. Missing is not an error.
func ClearDaemonUpdatePending() error {
	err := os.Remove(GetDaemonUpdatePendingPath())
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
