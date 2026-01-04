package model

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCleanLogFile_FileNotExists(t *testing.T) {
	freed, err := CleanLogFile("/nonexistent/path/file.log", 1024, false)
	if err != nil {
		t.Errorf("Should not return error for non-existent file: %v", err)
	}
	if freed != 0 {
		t.Errorf("Expected 0 bytes freed for non-existent file, got %d", freed)
	}
}

func TestCleanLogFile_BelowThreshold(t *testing.T) {
	// Create a temp file
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.log")
	content := []byte("small content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// File is smaller than threshold, should not be deleted
	threshold := int64(1024 * 1024) // 1MB threshold
	freed, err := CleanLogFile(testFile, threshold, false)
	if err != nil {
		t.Errorf("CleanLogFile failed: %v", err)
	}
	if freed != 0 {
		t.Errorf("Expected 0 bytes freed (below threshold), got %d", freed)
	}

	// Verify file still exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("File should not be deleted when below threshold")
	}
}

func TestCleanLogFile_AboveThreshold(t *testing.T) {
	// Create a temp file
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.log")
	// Create a file larger than threshold
	content := make([]byte, 2048)
	for i := range content {
		content[i] = 'x'
	}
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// File is larger than threshold, should be deleted
	threshold := int64(1024) // 1KB threshold
	freed, err := CleanLogFile(testFile, threshold, false)
	if err != nil {
		t.Errorf("CleanLogFile failed: %v", err)
	}
	if freed != 2048 {
		t.Errorf("Expected 2048 bytes freed, got %d", freed)
	}

	// Verify file was deleted
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("File should be deleted when above threshold")
	}
}

func TestCleanLogFile_ForceDelete(t *testing.T) {
	// Create a temp file
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.log")
	content := []byte("small content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// File is smaller than threshold, but force=true
	threshold := int64(1024 * 1024) // 1MB threshold
	freed, err := CleanLogFile(testFile, threshold, true)
	if err != nil {
		t.Errorf("CleanLogFile failed: %v", err)
	}
	if freed != int64(len(content)) {
		t.Errorf("Expected %d bytes freed, got %d", len(content), freed)
	}

	// Verify file was deleted
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("File should be deleted when force=true")
	}
}

func TestCleanLargeLogFiles(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir

	// Create log files
	logFile := GetLogFilePath()
	heartbeatFile := GetHeartbeatLogFilePath()
	syncFile := GetSyncPendingFilePath()

	// Create parent directories
	for _, f := range []string{logFile, heartbeatFile, syncFile} {
		dir := filepath.Dir(f)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir for %s: %v", f, err)
		}
	}

	// Create files with content above threshold
	largeContent := make([]byte, 2048)
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	for _, f := range []string{logFile, heartbeatFile} {
		if err := os.WriteFile(f, largeContent, 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", f, err)
		}
	}

	// Clean with 1KB threshold
	threshold := int64(1024)
	freed, err := CleanLargeLogFiles(threshold, false)
	if err != nil {
		t.Errorf("CleanLargeLogFiles failed: %v", err)
	}

	// Should have freed 2 files worth of data
	expectedFreed := int64(2 * 2048)
	if freed != expectedFreed {
		t.Errorf("Expected %d bytes freed, got %d", expectedFreed, freed)
	}
}

func TestCleanLargeLogFiles_NoFilesExist(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir

	// No files exist
	freed, err := CleanLargeLogFiles(1024, false)
	if err != nil {
		t.Errorf("CleanLargeLogFiles should not error when files don't exist: %v", err)
	}
	if freed != 0 {
		t.Errorf("Expected 0 bytes freed when no files exist, got %d", freed)
	}
}

func TestCleanLargeLogFiles_ForceDelete(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir

	// Create log file with small content
	logFile := GetLogFilePath()
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	smallContent := []byte("tiny")
	if err := os.WriteFile(logFile, smallContent, 0644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}

	// Clean with force=true (should delete regardless of size)
	threshold := int64(1024 * 1024) // Very high threshold
	freed, err := CleanLargeLogFiles(threshold, true)
	if err != nil {
		t.Errorf("CleanLargeLogFiles failed: %v", err)
	}

	if freed != int64(len(smallContent)) {
		t.Errorf("Expected %d bytes freed with force=true, got %d", len(smallContent), freed)
	}
}

func TestCleanDaemonLogFiles(t *testing.T) {
	// This function is platform-specific
	if runtime.GOOS != "darwin" {
		// On non-macOS, should return 0 and no error
		freed, err := CleanDaemonLogFiles(1024, false)
		if err != nil {
			t.Errorf("CleanDaemonLogFiles should not error on non-darwin: %v", err)
		}
		if freed != 0 {
			t.Errorf("Expected 0 bytes freed on non-darwin, got %d", freed)
		}
		return
	}

	// macOS-specific tests
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir

	// Create daemon log files
	logFile := GetDaemonLogFilePath()
	errFile := GetDaemonErrFilePath()

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		t.Fatalf("Failed to create logs dir: %v", err)
	}

	// Create files with content above threshold
	largeContent := make([]byte, 2048)
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	for _, f := range []string{logFile, errFile} {
		if err := os.WriteFile(f, largeContent, 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", f, err)
		}
	}

	// Clean with 1KB threshold
	threshold := int64(1024)
	freed, err := CleanDaemonLogFiles(threshold, false)
	if err != nil {
		t.Errorf("CleanDaemonLogFiles failed: %v", err)
	}

	// Should have freed 2 files worth of data
	expectedFreed := int64(2 * 2048)
	if freed != expectedFreed {
		t.Errorf("Expected %d bytes freed, got %d", expectedFreed, freed)
	}
}

func TestCleanDaemonLogFiles_NoFilesExist(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test on non-darwin platform")
	}

	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir

	// No files exist
	freed, err := CleanDaemonLogFiles(1024, false)
	if err != nil {
		t.Errorf("CleanDaemonLogFiles should not error when files don't exist: %v", err)
	}
	if freed != 0 {
		t.Errorf("Expected 0 bytes freed when no files exist, got %d", freed)
	}
}

func TestCleanLogFile_PermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	// Create a temp file in a directory we'll make unreadable
	tempDir, err := os.MkdirTemp("", "shelltime-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.log")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Make the directory read-only (prevents deletion)
	if err := os.Chmod(tempDir, 0555); err != nil {
		t.Fatalf("Failed to chmod dir: %v", err)
	}
	defer os.Chmod(tempDir, 0755) // Restore for cleanup

	// Try to clean the file with force=true
	_, err = CleanLogFile(testFile, 0, true)
	if err == nil {
		t.Error("Expected error when deleting file in read-only directory")
	}
}
