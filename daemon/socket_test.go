package daemon

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
)

func TestNewSocketHandler(t *testing.T) {
	config := &model.ShellTimeConfig{
		SocketPath: "/tmp/test-shelltime.sock",
	}
	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)

	handler := NewSocketHandler(config, ch)
	if handler == nil {
		t.Fatal("NewSocketHandler returned nil")
	}

	if handler.config != config {
		t.Error("Config not properly set")
	}

	if handler.channel != ch {
		t.Error("Channel not properly set")
	}

	if handler.stopChan == nil {
		t.Error("stopChan should be initialized")
	}

	if handler.ccInfoTimer == nil {
		t.Error("ccInfoTimer should be initialized")
	}
}

func TestSocketHandler_StartStop(t *testing.T) {
	// Create temp socket path
	tempDir, err := os.MkdirTemp("", "shelltime-socket-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")
	config := &model.ShellTimeConfig{
		SocketPath: socketPath,
	}
	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)

	handler := NewSocketHandler(config, ch)

	// Start the handler
	err = handler.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify socket file was created
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("Socket file was not created")
	}

	// Stop the handler
	handler.Stop()

	// Give it a moment to clean up
	time.Sleep(100 * time.Millisecond)

	// Verify socket file was removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket file should be removed after stop")
	}
}

func TestSocketHandler_StatusRequest(t *testing.T) {
	// Create temp socket path
	tempDir, err := os.MkdirTemp("", "shelltime-socket-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")
	config := &model.ShellTimeConfig{
		SocketPath: socketPath,
	}
	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)

	handler := NewSocketHandler(config, ch)

	err = handler.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer handler.Stop()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Connect to socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send status request
	msg := SocketMessage{
		Type: SocketMessageTypeStatus,
	}
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(msg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read response
	var response StatusResponse
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response
	if response.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
	if response.Platform == "" {
		t.Error("Platform should not be empty")
	}
	if response.Uptime == "" {
		t.Error("Uptime should not be empty")
	}
}

func TestSocketMessageType_Constants(t *testing.T) {
	testCases := []struct {
		msgType  SocketMessageType
		expected string
	}{
		{SocketMessageTypeSync, "sync"},
		{SocketMessageTypeHeartbeat, "heartbeat"},
		{SocketMessageTypeStatus, "status"},
		{SocketMessageTypeCCInfo, "cc_info"},
	}

	for _, tc := range testCases {
		if string(tc.msgType) != tc.expected {
			t.Errorf("Expected %s, got %s", tc.expected, tc.msgType)
		}
	}
}

func TestCCInfoTimeRange_Constants(t *testing.T) {
	testCases := []struct {
		timeRange CCInfoTimeRange
		expected  string
	}{
		{CCInfoTimeRangeToday, "today"},
		{CCInfoTimeRangeWeek, "week"},
		{CCInfoTimeRangeMonth, "month"},
	}

	for _, tc := range testCases {
		if string(tc.timeRange) != tc.expected {
			t.Errorf("Expected %s, got %s", tc.expected, tc.timeRange)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	testCases := []struct {
		duration time.Duration
		expected string
	}{
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m 5s"},
		{3665 * time.Second, "1h 1m 5s"},
		{90065 * time.Second, "1d 1h 1m 5s"},
		{0, "0s"},
		{30 * time.Minute, "30m 0s"},
		{2 * time.Hour, "2h 0m 0s"},
		{48 * time.Hour, "2d 0h 0m 0s"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := formatDuration(tc.duration)
			if result != tc.expected {
				t.Errorf("formatDuration(%v) = %s, expected %s", tc.duration, result, tc.expected)
			}
		})
	}
}

func TestSocketMessage_JSON(t *testing.T) {
	msg := SocketMessage{
		Type: SocketMessageTypeSync,
		Payload: map[string]interface{}{
			"key": "value",
		},
	}

	// Marshal
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded SocketMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != SocketMessageTypeSync {
		t.Errorf("Expected type %s, got %s", SocketMessageTypeSync, decoded.Type)
	}
}

func TestStatusResponse_JSON(t *testing.T) {
	response := StatusResponse{
		Version:   "1.0.0",
		StartedAt: time.Now(),
		Uptime:    "1h 30m 0s",
		GoVersion: "go1.21.0",
		Platform:  "linux/amd64",
	}

	// Marshal
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded StatusResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Version != response.Version {
		t.Errorf("Version mismatch: expected %s, got %s", response.Version, decoded.Version)
	}
	if decoded.GoVersion != response.GoVersion {
		t.Errorf("GoVersion mismatch: expected %s, got %s", response.GoVersion, decoded.GoVersion)
	}
}

func TestCCInfoRequest_JSON(t *testing.T) {
	request := CCInfoRequest{
		TimeRange: CCInfoTimeRangeToday,
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded CCInfoRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.TimeRange != CCInfoTimeRangeToday {
		t.Errorf("TimeRange mismatch: expected %s, got %s", CCInfoTimeRangeToday, decoded.TimeRange)
	}
}

func TestCCInfoResponse_JSON(t *testing.T) {
	now := time.Now()
	response := CCInfoResponse{
		TotalCostUSD: 1.23,
		TimeRange:    "today",
		CachedAt:     now,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded CCInfoResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.TotalCostUSD != 1.23 {
		t.Errorf("TotalCostUSD mismatch: expected 1.23, got %f", decoded.TotalCostUSD)
	}
	if decoded.TimeRange != "today" {
		t.Errorf("TimeRange mismatch: expected today, got %s", decoded.TimeRange)
	}
}

func TestSocketHandler_MultipleConnections(t *testing.T) {
	// Create temp socket path
	tempDir, err := os.MkdirTemp("", "shelltime-socket-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")
	config := &model.ShellTimeConfig{
		SocketPath: socketPath,
	}
	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)

	handler := NewSocketHandler(config, ch)

	err = handler.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer handler.Stop()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Make multiple concurrent connections
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func() {
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				done <- false
				return
			}
			defer conn.Close()

			msg := SocketMessage{Type: SocketMessageTypeStatus}
			encoder := json.NewEncoder(conn)
			encoder.Encode(msg)

			var response StatusResponse
			decoder := json.NewDecoder(conn)
			if err := decoder.Decode(&response); err != nil {
				done <- false
				return
			}

			done <- response.Platform != ""
		}()
	}

	// Wait for all connections
	successCount := 0
	for i := 0; i < 3; i++ {
		if <-done {
			successCount++
		}
	}

	if successCount != 3 {
		t.Errorf("Expected 3 successful connections, got %d", successCount)
	}
}
