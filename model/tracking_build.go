package model

import (
	"context"
	"sync/atomic"
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
//
// Meta.OS and Meta.OSVersion are left empty: reading them execs a program
// (lsb_release, sw_vers), and most calls stop below FlushCount without sending.
// Callers fill them with TrackingMetaData.WithOSInfo right before sending.
func BuildTrackingData(ctx context.Context, store CommandStore, config ShellTimeConfig) (TrackingBuildResult, error) {
	ctx, span := modelTracer.Start(ctx, "buildTrackingData")
	defer span.End()

	var res TrackingBuildResult

	var postCommands []*Command
	var cursor time.Time
	var noCursorExist bool

	pending, ok, err := pendingPostCommands(ctx, store)
	if err != nil {
		return res, err
	}
	if ok {
		// Without any post command the build ends here, as it does below
		if !pending.any {
			return res, nil
		}
		postCommands, cursor, noCursorExist = pending.posts, pending.cursor, pending.noCursorExist
	} else {
		postCommands, err = store.GetPostCommands(ctx)
		if err != nil {
			return res, err
		}
		if len(postCommands) == 0 {
			return res, nil
		}

		cursor, noCursorExist, err = store.GetLastCursor(ctx)
		if err != nil {
			return res, err
		}
	}
	res.Cursor = cursor
	res.NoCursorExist = noCursorExist

	preTree, err := store.GetPreTree(ctx)
	if err != nil {
		return res, err
	}

	meta := TrackingMetaData{
		CliEngine: store.Engine(),
	}

	trackingData := make([]TrackingData, 0)
	latest := cursor

	for _, postCommand := range postCommands {
		if postCommand == nil {
			continue
		}

		recordingTime := postCommand.RecordingTime
		// The cursor is the recording time of the last command included in a
		// successful sync. Equality means that command was already sent; Before
		// alone would upload it again on every later flush (the file store is
		// not pruned on the daemon-less path).
		if !recordingTime.After(cursor) {
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

		// The start falls back to the end, like BuildListedCommands: when no pre
		// started at or before this post (its pre was lost, or the clock stepped
		// back in between), FindClosestCommand returns an empty command, whose
		// zero time would be uploaded as a start in year 1.
		td := TrackingData{
			SessionID:     postCommand.SessionID,
			Command:       postCommand.Command,
			StartTime:     postCommand.Time.Unix(),
			StartTimeNano: postCommand.Time.UnixNano(),
			EndTime:       postCommand.Time.Unix(),
			EndTimeNano:   postCommand.Time.UnixNano(),
			Result:        postCommand.Result,
			PPID:          postCommand.PPID,
			ViaSSH:        postCommand.ViaSSH,
		}

		if config.DataMasking != nil && *config.DataMasking {
			td.Command = MaskSensitiveTokens(td.Command)
		}

		if closestPreCommand != nil && !closestPreCommand.IsNil() {
			td.StartTime = closestPreCommand.Time.Unix()
			td.StartTimeNano = closestPreCommand.Time.UnixNano()
			if pre := closestPreCommand.ViaSSH; pre != nil && (td.ViaSSH == nil || *pre) {
				td.ViaSSH = pre
			}
		}

		trackingData = append(trackingData, td)
	}

	res.Data = trackingData
	res.Meta = meta
	res.LatestRecordingTime = latest
	return res, nil
}

// postsAfterCursorStore is a store that can hand out the post commands recorded after a
// cursor without decoding the ones at or before it, and say whether it holds any post
// command at all.
type postsAfterCursorStore interface {
	getPostCommandsAfter(ctx context.Context, cursor time.Time) (posts []*Command, any bool, err error)
}

// pendingPosts is what BuildTrackingData's shortcut read from such a store.
type pendingPosts struct {
	// posts are the post commands recorded after the cursor
	posts []*Command
	// any is false when the store holds no post command at all, synced or not
	any           bool
	cursor        time.Time
	noCursorExist bool
}

// pendingPostCommands is BuildTrackingData's shortcut for such a store: the post commands
// after the store's cursor, and that cursor. BuildTrackingData skips every other post
// command before it looks at anything but its recording time, so the result is the same
// as with all of them.
//
// ok is false when the shortcut doesn't apply and the caller must read everything the
// plain way: the store has no such method, or its cursor can't be read (the plain way
// reports that error, in its own order).
func pendingPostCommands(ctx context.Context, store CommandStore) (pending pendingPosts, ok bool, err error) {
	fast, isFast := store.(postsAfterCursorStore)
	if !isFast {
		return pendingPosts{}, false, nil
	}
	cursor, noCursorExist, err := store.GetLastCursor(ctx)
	if err != nil {
		return pendingPosts{}, false, nil
	}
	posts, any, err := fast.getPostCommandsAfter(ctx, cursor)
	if err != nil {
		return pendingPosts{}, false, err
	}
	return pendingPosts{posts: posts, any: any, cursor: cursor, noCursorExist: noCursorExist}, true, nil
}

// osInfo is the first OS lookup that worked. The daemon sends a batch on every
// flush, and the OS and its version don't change while it runs, so the
// program is executed once. A failed lookup isn't kept, so the next send
// tries again.
var osInfo atomic.Pointer[SysInfo]

// WithOSInfo returns m with the OS and its version filled in, "unknown" when
// they can't be read. BuildTrackingData leaves them to the caller that sends.
func (m TrackingMetaData) WithOSInfo() TrackingMetaData {
	sysInfo := osInfo.Load()
	if sysInfo == nil {
		info, err := GetOSAndVersion()
		if err != nil {
			info = &SysInfo{Os: "unknown", Version: "unknown"}
		} else {
			osInfo.Store(info)
		}
		sysInfo = info
	}
	m.OS = sysInfo.Os
	m.OSVersion = sysInfo.Version
	return m
}
