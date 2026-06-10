package model

import (
	"context"
	"time"
)

// TrackingBuildResult is the outcome of assembling pending post commands into a
// server-ready tracking payload.
type TrackingBuildResult struct {
	Data                []TrackingData
	Meta                TrackingMetaData
	LatestRecordingTime time.Time
	Cursor              time.Time
	NoCursorExist       bool
}

// BuildTrackingData assembles the tracking payload for all post commands newer
// than the store's cursor, pairing each with its closest pre command. It is the
// shared assembly used by both the CLI fallback path and the daemon's bolt path
// (previously inlined in commands.trySyncLocalToServer).
//
// It performs no network IO and does not advance the cursor; callers decide
// whether to flush (FlushCount), send, then SetCursor + Prune.
func BuildTrackingData(ctx context.Context, store CommandStore, config ShellTimeConfig) (TrackingBuildResult, error) {
	ctx, span := modelTracer.Start(ctx, "buildTrackingData")
	defer span.End()

	var res TrackingBuildResult

	postCommands, err := store.GetPostCommands(ctx)
	if err != nil {
		return res, err
	}
	if len(postCommands) == 0 {
		return res, nil
	}

	cursor, noCursorExist, err := store.GetLastCursor(ctx)
	if err != nil {
		return res, err
	}
	res.Cursor = cursor
	res.NoCursorExist = noCursorExist

	preTree, err := store.GetPreTree(ctx)
	if err != nil {
		return res, err
	}

	sysInfo, err := GetOSAndVersion()
	if err != nil {
		sysInfo = &SysInfo{Os: "unknown", Version: "unknown"}
	}

	meta := TrackingMetaData{
		OS:        sysInfo.Os,
		OSVersion: sysInfo.Version,
	}

	trackingData := make([]TrackingData, 0)
	latest := cursor

	for _, postCommand := range postCommands {
		if postCommand == nil {
			continue
		}

		recordingTime := postCommand.RecordingTime
		if recordingTime.Before(cursor) {
			continue
		}
		if recordingTime.After(latest) {
			latest = recordingTime
		}

		key := postCommand.GetUniqueKey()
		preCommands, ok := preTree[key]
		if !ok {
			continue
		}

		if meta.Hostname == "" {
			meta.Hostname = postCommand.Hostname
		}
		if meta.Shell == "" {
			meta.Shell = postCommand.Shell
		}
		if meta.Username == "" {
			meta.Username = postCommand.Username
		}

		if ShouldExcludeCommand(postCommand.Command, config.Exclude) {
			continue
		}

		closestPreCommand := postCommand.FindClosestCommand(preCommands, false)

		td := TrackingData{
			SessionID:   postCommand.SessionID,
			Command:     postCommand.Command,
			EndTime:     postCommand.Time.Unix(),
			EndTimeNano: postCommand.Time.UnixNano(),
			Result:      postCommand.Result,
			PPID:        postCommand.PPID,
		}

		if config.DataMasking != nil && *config.DataMasking {
			td.Command = MaskSensitiveTokens(td.Command)
		}

		if closestPreCommand != nil {
			td.StartTime = closestPreCommand.Time.Unix()
			td.StartTimeNano = closestPreCommand.Time.UnixNano()
		}

		trackingData = append(trackingData, td)
	}

	res.Data = trackingData
	res.Meta = meta
	res.LatestRecordingTime = latest
	return res, nil
}
