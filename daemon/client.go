package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"time"

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

// RequestCCInfo requests CC info (cost data and git info) from the daemon
func RequestCCInfo(socketPath string, timeRange CCInfoTimeRange, workingDir string, timeout time.Duration) (*CCInfoResponse, error) {
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
			TimeRange:        timeRange,
			WorkingDirectory: workingDir,
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
