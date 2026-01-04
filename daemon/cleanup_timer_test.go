package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
)

func TestNewCleanupTimerService(t *testing.T) {
	config := model.ShellTimeConfig{
		LogCleanup: model.LogCleanupConfig{
			ThresholdMB: 100,
		},
	}

	service := NewCleanupTimerService(config)
	if service == nil {
		t.Fatal("NewCleanupTimerService returned nil")
	}

	if service.config.LogCleanup.ThresholdMB != 100 {
		t.Errorf("Expected ThresholdMB 100, got %d", service.config.LogCleanup.ThresholdMB)
	}

	if service.stopChan == nil {
		t.Error("stopChan should be initialized")
	}
}

func TestCleanupTimerService_StartStop(t *testing.T) {
	config := model.ShellTimeConfig{
		LogCleanup: model.LogCleanupConfig{
			ThresholdMB: 100,
		},
	}

	service := NewCleanupTimerService(config)

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

func TestCleanupTimerService_StopWithoutStart(t *testing.T) {
	config := model.ShellTimeConfig{
		LogCleanup: model.LogCleanupConfig{
			ThresholdMB: 100,
		},
	}

	service := NewCleanupTimerService(config)

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop should not panic: %v", r)
		}
	}()

	service.Stop()
}

func TestCleanupTimerService_ContextCancellation(t *testing.T) {
	config := model.ShellTimeConfig{
		LogCleanup: model.LogCleanupConfig{
			ThresholdMB: 100,
		},
	}

	service := NewCleanupTimerService(config)

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

func TestCleanupInterval_Constant(t *testing.T) {
	expected := 24 * time.Hour
	if CleanupInterval != expected {
		t.Errorf("Expected CleanupInterval to be 24h, got %v", CleanupInterval)
	}
}

func TestCleanupTimerService_Cleanup(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := model.COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		model.COMMAND_BASE_STORAGE_FOLDER = origBase
	}()

	model.COMMAND_BASE_STORAGE_FOLDER = tempDir

	config := model.ShellTimeConfig{
		LogCleanup: model.LogCleanupConfig{
			ThresholdMB: 1, // 1MB threshold (1024 * 1024 bytes)
		},
	}

	service := NewCleanupTimerService(config)

	// Create a log file larger than threshold
	logFile := model.GetLogFilePath()
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	// Create a file larger than 1MB
	largeContent := make([]byte, 2*1024*1024) // 2MB
	for i := range largeContent {
		largeContent[i] = 'x'
	}
	if err := os.WriteFile(logFile, largeContent, 0644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Fatal("Log file was not created")
	}

	// Run cleanup
	ctx := context.Background()
	service.cleanup(ctx)

	// Verify file was deleted (exceeded threshold)
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Error("Log file should have been deleted")
	}
}

func TestCleanupTimerService_CleanupBelowThreshold(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := model.COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		model.COMMAND_BASE_STORAGE_FOLDER = origBase
	}()

	model.COMMAND_BASE_STORAGE_FOLDER = tempDir

	config := model.ShellTimeConfig{
		LogCleanup: model.LogCleanupConfig{
			ThresholdMB: 10, // 10MB threshold
		},
	}

	service := NewCleanupTimerService(config)

	// Create a small log file
	logFile := model.GetLogFilePath()
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	smallContent := []byte("small log content")
	if err := os.WriteFile(logFile, smallContent, 0644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}

	// Run cleanup
	ctx := context.Background()
	service.cleanup(ctx)

	// Verify file still exists (below threshold)
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("Log file should NOT have been deleted (below threshold)")
	}
}
