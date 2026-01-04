package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
)

func TestNewHeartbeatResyncService(t *testing.T) {
	config := model.ShellTimeConfig{}

	service := NewHeartbeatResyncService(config)
	if service == nil {
		t.Fatal("NewHeartbeatResyncService returned nil")
	}

	if service.stopChan == nil {
		t.Error("stopChan should be initialized")
	}
}

func TestHeartbeatResyncService_StartStop(t *testing.T) {
	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if service.ticker == nil {
		t.Error("Ticker should be initialized after Start")
	}

	// Stop the service
	service.Stop()

	// Give it a moment to stop
	time.Sleep(50 * time.Millisecond)
}

func TestHeartbeatResyncService_StopWithoutStart(t *testing.T) {
	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop should not panic: %v", r)
		}
	}()

	service.Stop()
}

func TestHeartbeatResyncService_ContextCancellation(t *testing.T) {
	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	ctx, cancel := context.WithCancel(context.Background())

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Cancel context should trigger stop
	cancel()

	// Give it time to process
	time.Sleep(50 * time.Millisecond)
}

func TestHeartbeatResyncInterval_Constant(t *testing.T) {
	expected := 30 * time.Minute
	if HeartbeatResyncInterval != expected {
		t.Errorf("Expected HeartbeatResyncInterval to be 30m, got %v", HeartbeatResyncInterval)
	}
}

func TestHeartbeatResyncService_RewriteLogFile_EmptyLines(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-resync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "heartbeat.log")

	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	// Test with empty lines - should remove file
	err = service.rewriteLogFile(logFile, []string{})
	if err != nil {
		t.Fatalf("rewriteLogFile failed: %v", err)
	}

	// File should not exist
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Error("File should be removed when lines are empty")
	}
}

func TestHeartbeatResyncService_RewriteLogFile_WithLines(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-resync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "heartbeat.log")

	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	lines := []string{
		`{"heartbeats":[{"heartbeatId":"1"}]}`,
		`{"heartbeats":[{"heartbeatId":"2"}]}`,
	}

	err = service.rewriteLogFile(logFile, lines)
	if err != nil {
		t.Fatalf("rewriteLogFile failed: %v", err)
	}

	// Verify file content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	for _, line := range lines {
		if !contains(string(content), line) {
			t.Errorf("Expected file to contain: %s", line)
		}
	}
}

func TestHeartbeatResyncService_RewriteLogFile_AtomicRename(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-resync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "heartbeat.log")

	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	lines := []string{`{"test":"data"}`}

	err = service.rewriteLogFile(logFile, lines)
	if err != nil {
		t.Fatalf("rewriteLogFile failed: %v", err)
	}

	// Temp file should not exist after atomic rename
	tempFile := logFile + ".tmp"
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Error("Temp file should be removed after atomic rename")
	}
}

func TestHeartbeatResyncService_ResyncNoFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-resync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	// This should not panic when file doesn't exist
	ctx := context.Background()
	service.resync(ctx)
}

func TestHeartbeatResyncService_ResyncEmptyFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-resync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create empty heartbeat log file
	logDir := filepath.Join(tempDir, ".shelltime")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	logFile := filepath.Join(logDir, "coding-heartbeat.data.log")
	if err := os.WriteFile(logFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty log file: %v", err)
	}

	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	// This should not panic with empty file
	ctx := context.Background()
	service.resync(ctx)
}

func TestHeartbeatResyncService_ResyncInvalidJSON(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-resync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create heartbeat log file with invalid JSON
	logDir := filepath.Join(tempDir, ".shelltime")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	logFile := filepath.Join(logDir, "coding-heartbeat.data.log")
	invalidContent := "not valid json\n{also not valid}\n"
	if err := os.WriteFile(logFile, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	config := model.ShellTimeConfig{}
	service := NewHeartbeatResyncService(config)

	// This should not panic with invalid JSON (lines are discarded)
	ctx := context.Background()
	service.resync(ctx)

	// File should be removed since all lines were invalid
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Error("File with all invalid lines should be removed")
	}
}

func TestHeartbeatPayload_JSON(t *testing.T) {
	payload := model.HeartbeatPayload{
		Heartbeats: []model.HeartbeatData{
			{
				HeartbeatID: "test-id-1",
				Entity:      "/path/to/file.go",
				Time:        time.Now().Unix(),
				Project:     "test-project",
			},
		},
	}

	// Marshal
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded model.HeartbeatPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Heartbeats) != 1 {
		t.Errorf("Expected 1 heartbeat, got %d", len(decoded.Heartbeats))
	}

	if decoded.Heartbeats[0].HeartbeatID != "test-id-1" {
		t.Errorf("HeartbeatID mismatch: expected test-id-1, got %s", decoded.Heartbeats[0].HeartbeatID)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
