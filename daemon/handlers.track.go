package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/malamtime/cli/model"
)

// newFallbackStore builds the store used for track events when the bolt engine
// is disabled (commandStore is nil). It is a var so tests can substitute an
// in-memory store without touching the filesystem.
var newFallbackStore = model.NewFileStore

// trackStore returns the active command store for the daemon. The CLI forwards
// every track event to the daemon and lets it decide where to persist: the
// daemon-owned bolt store when the bolt engine is enabled, otherwise the txt
// file store (both satisfy model.CommandStore).
func trackStore() model.CommandStore {
	if commandStore != nil {
		return commandStore
	}
	return newFallbackStore()
}

func parseTrackEvent(payload interface{}) (model.Command, time.Time, error) {
	pb, err := json.Marshal(payload)
	if err != nil {
		return model.Command{}, time.Time{}, err
	}
	var ev TrackEventPayload
	if err := json.Unmarshal(pb, &ev); err != nil {
		return model.Command{}, time.Time{}, err
	}
	recordingTime := time.Unix(0, ev.RecordingTimeNano)
	return ev.Command, recordingTime, nil
}

// handlePubSubTrackPre persists a pre-execution command to the active bucket,
// unless config marks the command as excluded.
func handlePubSubTrackPre(ctx context.Context, payload interface{}) error {
	cmd, recordingTime, err := parseTrackEvent(payload)
	if err != nil {
		slog.Error("Failed to parse track_pre payload", slog.Any("err", err))
		return err
	}

	cfg, err := stConfig.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("Failed to read config in track_pre handler", slog.Any("err", err))
		return err
	}
	if model.ShouldExcludeCommand(cmd.Command, cfg.Exclude) {
		slog.Debug("Command excluded by pattern", slog.String("command", cmd.Command))
		return nil
	}

	return trackStore().SavePre(ctx, cmd, recordingTime)
}

// handlePubSubTrackPost persists a post-execution command, then runs the
// flush/sync/cursor/prune cycle against the active store — the daemon-side
// equivalent of commands.trySyncLocalToServer.
func handlePubSubTrackPost(ctx context.Context, payload interface{}) error {
	cmd, recordingTime, err := parseTrackEvent(payload)
	if err != nil {
		slog.Error("Failed to parse track_post payload", slog.Any("err", err))
		return err
	}

	cfg, err := stConfig.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("Failed to read config in track_post handler", slog.Any("err", err))
		return err
	}
	if model.ShouldExcludeCommand(cmd.Command, cfg.Exclude) {
		slog.Debug("Command excluded by pattern", slog.String("command", cmd.Command))
		return nil
	}

	store := trackStore()
	if err := store.SavePost(ctx, cmd, cmd.Result, recordingTime); err != nil {
		return err
	}

	result, err := model.BuildTrackingData(ctx, store, cfg)
	if err != nil {
		return err
	}
	if len(result.Data) == 0 {
		return nil
	}

	// Respect FlushCount, allowing the very first batch through (no cursor yet).
	if !result.NoCursorExist && len(result.Data) < cfg.FlushCount {
		slog.Debug("not enough data to flush", slog.Int("current", len(result.Data)), slog.Int("flushCount", cfg.FlushCount))
		return nil
	}

	args := model.PostTrackArgs{
		CursorID: result.LatestRecordingTime.UnixNano(),
		Data:     result.Data,
		Meta:     result.Meta,
	}

	if err := sendTrackArgsToServer(ctx, args); err != nil {
		slog.Error("Failed to send tracking data from command store", slog.Any("err", err))
		// Leave the data in the store; a later post will retry. Do not advance cursor.
		return err
	}

	if err := store.SetCursor(ctx, result.LatestRecordingTime); err != nil {
		slog.Error("Failed to advance cursor", slog.Any("err", err))
		return err
	}
	if err := store.Prune(ctx, result.LatestRecordingTime); err != nil {
		slog.Warn("Failed to prune synced commands", slog.Any("err", err))
	}
	return nil
}
