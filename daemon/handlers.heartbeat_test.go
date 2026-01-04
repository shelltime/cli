package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
)

func TestSaveHeartbeatToFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-heartbeat-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create the .shelltime directory
	shelltimeDir := filepath.Join(tempDir, ".shelltime")
	if err := os.MkdirAll(shelltimeDir, 0755); err != nil {
		t.Fatalf("Failed to create shelltime dir: %v", err)
	}

	payload := model.HeartbeatPayload{
		Heartbeats: []model.HeartbeatData{
			{
				HeartbeatID: "test-id-1",
				Entity:      "/path/to/file.go",
				Time:        1234567890,
				Project:     "test-project",
			},
		},
	}

	err = saveHeartbeatToFile(payload)
	if err != nil {
		t.Fatalf("saveHeartbeatToFile failed: %v", err)
	}

	// Verify file was created
	logFile := filepath.Join(shelltimeDir, "coding-heartbeat.data.log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("Heartbeat log file was not created")
	}

	// Verify content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !contains(string(content), "test-id-1") {
		t.Error("Log file should contain heartbeat ID")
	}

	if !contains(string(content), "test-project") {
		t.Error("Log file should contain project name")
	}
}

func TestSaveHeartbeatToFile_Append(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-heartbeat-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create the .shelltime directory
	shelltimeDir := filepath.Join(tempDir, ".shelltime")
	if err := os.MkdirAll(shelltimeDir, 0755); err != nil {
		t.Fatalf("Failed to create shelltime dir: %v", err)
	}

	// Save first heartbeat
	payload1 := model.HeartbeatPayload{
		Heartbeats: []model.HeartbeatData{
			{HeartbeatID: "id-1", Entity: "file1.go", Time: 1234567890},
		},
	}
	err = saveHeartbeatToFile(payload1)
	if err != nil {
		t.Fatalf("First saveHeartbeatToFile failed: %v", err)
	}

	// Save second heartbeat
	payload2 := model.HeartbeatPayload{
		Heartbeats: []model.HeartbeatData{
			{HeartbeatID: "id-2", Entity: "file2.go", Time: 1234567891},
		},
	}
	err = saveHeartbeatToFile(payload2)
	if err != nil {
		t.Fatalf("Second saveHeartbeatToFile failed: %v", err)
	}

	// Verify both are in the file
	logFile := filepath.Join(shelltimeDir, "coding-heartbeat.data.log")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !contains(string(content), "id-1") {
		t.Error("Log file should contain first heartbeat ID")
	}

	if !contains(string(content), "id-2") {
		t.Error("Log file should contain second heartbeat ID")
	}
}

func TestSaveHeartbeatToFile_EmptyPayload(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-heartbeat-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create the .shelltime directory
	shelltimeDir := filepath.Join(tempDir, ".shelltime")
	if err := os.MkdirAll(shelltimeDir, 0755); err != nil {
		t.Fatalf("Failed to create shelltime dir: %v", err)
	}

	// Empty payload
	payload := model.HeartbeatPayload{
		Heartbeats: []model.HeartbeatData{},
	}

	err = saveHeartbeatToFile(payload)
	if err != nil {
		t.Fatalf("saveHeartbeatToFile with empty payload failed: %v", err)
	}

	// File should still be created (with empty heartbeats array)
	logFile := filepath.Join(shelltimeDir, "coding-heartbeat.data.log")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !contains(string(content), `"heartbeats":[]`) {
		t.Error("Log file should contain empty heartbeats array")
	}
}

func TestSaveHeartbeatToFile_DirectoryNotExists(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-heartbeat-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to a non-existent path
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Don't create the .shelltime directory - saveHeartbeatToFile should fail
	payload := model.HeartbeatPayload{
		Heartbeats: []model.HeartbeatData{
			{HeartbeatID: "test-id", Entity: "file.go", Time: 1234567890},
		},
	}

	err = saveHeartbeatToFile(payload)
	if err == nil {
		t.Error("Expected error when directory doesn't exist")
	}
}

func TestSaveHeartbeatToFile_MultipleHeartbeats(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-heartbeat-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set HOME to temp dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create the .shelltime directory
	shelltimeDir := filepath.Join(tempDir, ".shelltime")
	if err := os.MkdirAll(shelltimeDir, 0755); err != nil {
		t.Fatalf("Failed to create shelltime dir: %v", err)
	}

	// Payload with multiple heartbeats
	payload := model.HeartbeatPayload{
		Heartbeats: []model.HeartbeatData{
			{HeartbeatID: "id-1", Entity: "file1.go", Time: 1234567890, Project: "proj1"},
			{HeartbeatID: "id-2", Entity: "file2.go", Time: 1234567891, Project: "proj1"},
			{HeartbeatID: "id-3", Entity: "file3.go", Time: 1234567892, Project: "proj2"},
		},
	}

	err = saveHeartbeatToFile(payload)
	if err != nil {
		t.Fatalf("saveHeartbeatToFile failed: %v", err)
	}

	// Verify all heartbeats are in the file
	logFile := filepath.Join(shelltimeDir, "coding-heartbeat.data.log")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	for _, hb := range payload.Heartbeats {
		if !contains(string(content), hb.HeartbeatID) {
			t.Errorf("Log file should contain heartbeat ID: %s", hb.HeartbeatID)
		}
	}
}
