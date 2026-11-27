package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/malamtime/cli/agenthook"
	"github.com/malamtime/cli/model"
)

func IsSocketReady(ctx context.Context, socketPath string) bool {
	_, err := os.Stat(socketPath)
	return err == nil
}

func SendLocalDataToSocket(
	ctx context.Context,
	socketPath string,
	config model.ShellTimeConfig,
	cursor time.Time,
	trackingData []model.TrackingData,
	meta model.TrackingMetaData,
) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	data := SocketMessage{
		Type: SocketMessageTypeSync,
		Payload: model.PostTrackArgs{
			CursorID: cursor.UnixNano(),
			Data:     trackingData,
			Meta:     meta,
		},
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = conn.Write(encoded)
	if err != nil {
		return err
	}

	return nil
}

// SendTrackEvent sends a single raw command event (pre or post) to the daemon
// for persistence in its bolt store. Fire-and-forget, mirroring the latency
// profile of the txt-file append it replaces.
func SendTrackEvent(
	ctx context.Context,
	socketPath string,
	msgType SocketMessageType,
	cmd model.Command,
	recordingTime time.Time,
) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	data := SocketMessage{
		Type: msgType,
		Payload: TrackEventPayload{
			Command:           cmd,
			RecordingTimeNano: recordingTime.UnixNano(),
		},
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = conn.Write(encoded)
	return err
}

// SendSessionProject sends a session-to-project mapping to the daemon (fire-and-forget)
func SendSessionProject(socketPath string, sessionID, projectPath string) {
	conn, err := net.DialTimeout("unix", socketPath, 10*time.Millisecond)
	if err != nil {
		return
	}
	defer conn.Close()

	msg := SocketMessage{
		Type: SocketMessageTypeSessionProject,
		Payload: SessionProjectRequest{
			SessionID:   sessionID,
			ProjectPath: projectPath,
		},
	}

	json.NewEncoder(conn).Encode(msg)
}

// SendSessionPullRequests hands pull request URLs opened in a Claude Code
// session to the daemon, which sends them to the server. It returns the dial or
// write error so the caller can send them directly when no daemon is running.
func SendSessionPullRequests(socketPath string, sessionID string, urls []string) error {
	conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()

	msg := SocketMessage{
		Type: SocketMessageTypeSessionPullRequests,
		Payload: SessionPullRequestsRequest{
			SessionID: sessionID,
			URLs:      urls,
		},
	}

	return json.NewEncoder(conn).Encode(msg)
}

// agentHookDialTimeout bounds connecting to the daemon socket for an agent hook event: a
// listening daemon accepts at once, and a socket file left by a dead one refuses at once.
const agentHookDialTimeout = 20 * time.Millisecond

// SendAgentHookEvent hands one Cursor or Antigravity hook event to the daemon and waits for
// its acknowledgment, all within timeout. It returns an error when the daemon didn't take the
// event, so the caller can keep it itself.
func SendAgentHookEvent(socketPath string, ev agenthook.Event, timeout time.Duration) error {
	conn, err := net.DialTimeout("unix", socketPath, min(agentHookDialTimeout, timeout))
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}

	msg := SocketMessage{Type: SocketMessageTypeAgentHook, Payload: ev}
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return err
	}
	var ack struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(conn).Decode(&ack); err != nil {
		return fmt.Errorf("no acknowledgment from the daemon: %w", err)
	}
	if ack.Status != "ok" {
		return fmt.Errorf("the daemon didn't take the agent hook event (status %q)", ack.Status)
	}
	return nil
}

// RequestListCommands asks the daemon for the locally buffered commands (used
// by `shelltime ls` in bolt mode, since the CLI can't open the locked DB).
func RequestListCommands(socketPath string, timeout time.Duration) (*ListCommandsResponse, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	msg := SocketMessage{Type: SocketMessageTypeListCommands}
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return nil, err
	}

	var response ListCommandsResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return nil, err
	}
	return &response, nil
}

// RequestCCInfo requests CC info (cost data and git info) from the daemon.
// claudeCodeVersion is forwarded so the daemon can use it in the Anthropic usage User-Agent.
func RequestCCInfo(socketPath string, timeRange CCInfoTimeRange, workingDir, claudeCodeVersion string, timeout time.Duration) (*CCInfoResponse, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Set read/write deadline
	conn.SetDeadline(time.Now().Add(timeout))

	// Send request
	msg := SocketMessage{
		Type: SocketMessageTypeCCInfo,
		Payload: CCInfoRequest{
			TimeRange:         timeRange,
			WorkingDirectory:  workingDir,
			ClaudeCodeVersion: claudeCodeVersion,
		},
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(msg); err != nil {
		return nil, err
	}

	// Read response
	var response CCInfoResponse
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}

	return &response, nil
}
