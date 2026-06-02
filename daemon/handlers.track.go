package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/malamtime/cli/model"
)

// errNoCommandStore is returned when a track event arrives but the bolt store
// was not initialized (bolt engine disabled). The CLI only emits track events
// when bolt is enabled, so this indicates a misconfiguration.
var errNoCommandStore = errors.New("command store not initialized; bolt storage engine is disabled")

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

// handlePubSubTrackPre persists a pre-execution command to the active bucket.
func handlePubSubTrackPre(ctx context.Context, payload interface{}) error {
	if commandStore == nil {
		return errNoCommandStore
	}
	cmd, recordingTime, err := parseTrackEvent(payload)
	if err != nil {
		slog.Error("Failed to parse track_pre payload", slog.Any("err", err))
		return err
	}
	return commandStore.SavePre(ctx, cmd, recordingTime)
}

// handlePubSubTrackPost persists a post-execution command, then runs the
// flush/sync/cursor/prune cycle against the bolt store — the daemon-side
// equivalent of commands.trySyncLocalToServer.
func handlePubSubTrackPost(ctx context.Context, payload interface{}) error {
	if commandStore == nil {
		return errNoCommandStore
	}
	cmd, recordingTime, err := parseTrackEvent(payload)
	if err != nil {
		slog.Error("Failed to parse track_post payload", slog.Any("err", err))
		return err
	}

	if err := commandStore.SavePost(ctx, cmd, cmd.Result, recordingTime); err != nil {
		return err
	}

	cfg, err := stConfig.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("Failed to read config in track_post handler", slog.Any("err", err))
		return err
	}

	result, err := model.BuildTrackingData(ctx, commandStore, cfg)
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
		slog.Error("Failed to send tracking data from bolt store", slog.Any("err", err))
		// Leave the data in bolt; a later post will retry. Do not advance cursor.
		return err
	}

	if err := commandStore.SetCursor(ctx, result.LatestRecordingTime); err != nil {
		slog.Error("Failed to advance cursor", slog.Any("err", err))
		return err
	}
	if err := commandStore.Prune(ctx, result.LatestRecordingTime); err != nil {
		slog.Warn("Failed to prune synced commands", slog.Any("err", err))
	}
	return nil
}
