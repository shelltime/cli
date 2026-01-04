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

// RequestCCInfo requests CC info (cost data) from the daemon
func RequestCCInfo(socketPath string, timeRange CCInfoTimeRange, timeout time.Duration) (*CCInfoResponse, error) {
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
			TimeRange: timeRange,
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
