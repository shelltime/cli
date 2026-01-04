package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
)

func TestIsSocketReady(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-client-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")
	ctx := context.Background()

	// Test non-existent socket
	if IsSocketReady(ctx, socketPath) {
		t.Error("Expected false for non-existent socket")
	}

	// Create the socket file
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer listener.Close()

	// Test existing socket
	if !IsSocketReady(ctx, socketPath) {
		t.Error("Expected true for existing socket")
	}
}

func TestSendLocalDataToSocket(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-client-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")

	// Create a mock server
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer listener.Close()

	// Start accepting connections in background
	received := make(chan *SocketMessage, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var msg SocketMessage
		decoder := json.NewDecoder(conn)
		if err := decoder.Decode(&msg); err != nil {
			return
		}
		received <- &msg
	}()

	ctx := context.Background()
	config := model.ShellTimeConfig{}
	cursor := time.Now()
	trackingData := []model.TrackingData{
		{Command: "ls -la"},
	}
	meta := model.TrackingMetaData{
		OS: "linux",
	}

	err = SendLocalDataToSocket(ctx, socketPath, config, cursor, trackingData, meta)
	if err != nil {
		t.Fatalf("SendLocalDataToSocket failed: %v", err)
	}

	// Wait for message to be received
	select {
	case msg := <-received:
		if msg.Type != SocketMessageTypeSync {
			t.Errorf("Expected message type %s, got %s", SocketMessageTypeSync, msg.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for message")
	}
}

func TestSendLocalDataToSocket_SocketNotExists(t *testing.T) {
	ctx := context.Background()
	config := model.ShellTimeConfig{}
	cursor := time.Now()

	err := SendLocalDataToSocket(ctx, "/nonexistent/socket.sock", config, cursor, nil, model.TrackingMetaData{})
	if err == nil {
		t.Error("Expected error when socket doesn't exist")
	}
}

func TestRequestCCInfo(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-client-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")

	// Create a mock server
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer listener.Close()

	// Start mock server
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read request
		var msg SocketMessage
		decoder := json.NewDecoder(conn)
		if err := decoder.Decode(&msg); err != nil {
			return
		}

		// Send response
		response := CCInfoResponse{
			TotalCostUSD: 5.50,
			TimeRange:    "today",
			CachedAt:     time.Now(),
		}
		encoder := json.NewEncoder(conn)
		encoder.Encode(response)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	response, err := RequestCCInfo(socketPath, CCInfoTimeRangeToday, 5*time.Second)
	if err != nil {
		t.Fatalf("RequestCCInfo failed: %v", err)
	}

	if response.TotalCostUSD != 5.50 {
		t.Errorf("Expected TotalCostUSD 5.50, got %f", response.TotalCostUSD)
	}
	if response.TimeRange != "today" {
		t.Errorf("Expected TimeRange 'today', got %s", response.TimeRange)
	}
}

func TestRequestCCInfo_Timeout(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-client-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")

	// Create a mock server that doesn't respond
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to create socket: %v", err)
	}
	defer listener.Close()

	// Start mock server that accepts but doesn't respond
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			// Keep connection open but don't respond
			time.Sleep(10 * time.Second)
			conn.Close()
		}
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	_, err = RequestCCInfo(socketPath, CCInfoTimeRangeToday, 100*time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestRequestCCInfo_SocketNotExists(t *testing.T) {
	_, err := RequestCCInfo("/nonexistent/socket.sock", CCInfoTimeRangeToday, 1*time.Second)
	if err == nil {
		t.Error("Expected error when socket doesn't exist")
	}
}

func TestRequestCCInfo_AllTimeRanges(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-client-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	socketPath := filepath.Join(tempDir, "test.sock")

	testRanges := []CCInfoTimeRange{
		CCInfoTimeRangeToday,
		CCInfoTimeRangeWeek,
		CCInfoTimeRangeMonth,
	}

	for _, timeRange := range testRanges {
		t.Run(string(timeRange), func(t *testing.T) {
			// Create a mock server
			listener, err := net.Listen("unix", socketPath)
			if err != nil {
				t.Fatalf("Failed to create socket: %v", err)
			}
			defer listener.Close()

			// Start mock server
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()

				var msg SocketMessage
				decoder := json.NewDecoder(conn)
				decoder.Decode(&msg)

				response := CCInfoResponse{
					TotalCostUSD: 1.0,
					TimeRange:    string(timeRange),
					CachedAt:     time.Now(),
				}
				encoder := json.NewEncoder(conn)
				encoder.Encode(response)
			}()

			time.Sleep(50 * time.Millisecond)

			response, err := RequestCCInfo(socketPath, timeRange, 5*time.Second)
			if err != nil {
				t.Fatalf("RequestCCInfo failed: %v", err)
			}

			if response.TimeRange != string(timeRange) {
				t.Errorf("Expected TimeRange %s, got %s", timeRange, response.TimeRange)
			}

			// Clean up for next iteration
			os.Remove(socketPath)
		})
	}
}
