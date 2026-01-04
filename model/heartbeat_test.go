package model

import (
	"encoding/json"
	"testing"
)

func TestHeartbeatPayload_JSON(t *testing.T) {
	payload := HeartbeatPayload{
		Heartbeats: []HeartbeatData{
			{
				HeartbeatID: "uuid-1234",
				Entity:      "/path/to/file.go",
				EntityType:  "file",
				Category:    "coding",
				Time:        1234567890,
				Project:     "my-project",
			},
		},
	}

	// Marshal
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded HeartbeatPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Heartbeats) != 1 {
		t.Fatalf("Expected 1 heartbeat, got %d", len(decoded.Heartbeats))
	}

	hb := decoded.Heartbeats[0]
	if hb.HeartbeatID != "uuid-1234" {
		t.Errorf("HeartbeatID mismatch: expected uuid-1234, got %s", hb.HeartbeatID)
	}
	if hb.Entity != "/path/to/file.go" {
		t.Errorf("Entity mismatch")
	}
	if hb.EntityType != "file" {
		t.Errorf("EntityType mismatch")
	}
	if hb.Category != "coding" {
		t.Errorf("Category mismatch")
	}
	if hb.Time != 1234567890 {
		t.Errorf("Time mismatch")
	}
	if hb.Project != "my-project" {
		t.Errorf("Project mismatch")
	}
}

func TestHeartbeatData_AllFields(t *testing.T) {
	lines := 100
	lineNum := 50
	cursor := 25

	hb := HeartbeatData{
		HeartbeatID:     "test-id",
		Entity:          "/home/user/project/main.go",
		EntityType:      "file",
		Category:        "coding",
		Time:            1234567890,
		Project:         "test-project",
		ProjectRootPath: "/home/user/project",
		Branch:          "main",
		Language:        "go",
		Lines:           &lines,
		LineNumber:      &lineNum,
		CursorPosition:  &cursor,
		Editor:          "vscode",
		EditorVersion:   "1.85.0",
		Plugin:          "shelltime",
		PluginVersion:   "1.0.0",
		Machine:         "workstation",
		OS:              "linux",
		OSVersion:       "ubuntu 22.04",
		IsWrite:         true,
	}

	// Marshal
	data, err := json.Marshal(hb)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded HeartbeatData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify all fields
	if decoded.HeartbeatID != hb.HeartbeatID {
		t.Errorf("HeartbeatID mismatch")
	}
	if decoded.Entity != hb.Entity {
		t.Errorf("Entity mismatch")
	}
	if decoded.Branch != hb.Branch {
		t.Errorf("Branch mismatch")
	}
	if decoded.Language != hb.Language {
		t.Errorf("Language mismatch")
	}
	if decoded.Lines == nil || *decoded.Lines != 100 {
		t.Errorf("Lines mismatch")
	}
	if decoded.LineNumber == nil || *decoded.LineNumber != 50 {
		t.Errorf("LineNumber mismatch")
	}
	if decoded.CursorPosition == nil || *decoded.CursorPosition != 25 {
		t.Errorf("CursorPosition mismatch")
	}
	if decoded.Editor != hb.Editor {
		t.Errorf("Editor mismatch")
	}
	if decoded.EditorVersion != hb.EditorVersion {
		t.Errorf("EditorVersion mismatch")
	}
	if decoded.Plugin != hb.Plugin {
		t.Errorf("Plugin mismatch")
	}
	if decoded.PluginVersion != hb.PluginVersion {
		t.Errorf("PluginVersion mismatch")
	}
	if decoded.Machine != hb.Machine {
		t.Errorf("Machine mismatch")
	}
	if decoded.OS != hb.OS {
		t.Errorf("OS mismatch")
	}
	if decoded.OSVersion != hb.OSVersion {
		t.Errorf("OSVersion mismatch")
	}
	if decoded.IsWrite != true {
		t.Errorf("IsWrite mismatch")
	}
}

func TestHeartbeatData_OptionalFields(t *testing.T) {
	// Minimal heartbeat with only required fields
	hb := HeartbeatData{
		HeartbeatID: "minimal-id",
		Entity:      "/file.txt",
		Time:        1234567890,
	}

	data, err := json.Marshal(hb)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded HeartbeatData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Optional fields should be nil or empty
	if decoded.Lines != nil {
		t.Error("Lines should be nil")
	}
	if decoded.LineNumber != nil {
		t.Error("LineNumber should be nil")
	}
	if decoded.CursorPosition != nil {
		t.Error("CursorPosition should be nil")
	}
	if decoded.EntityType != "" {
		t.Error("EntityType should be empty")
	}
	if decoded.Category != "" {
		t.Error("Category should be empty")
	}
}

func TestHeartbeatResponse_JSON(t *testing.T) {
	resp := HeartbeatResponse{
		Success:   true,
		Processed: 10,
		Errors:    2,
		Message:   "Partially processed",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded HeartbeatResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Success != true {
		t.Error("Success mismatch")
	}
	if decoded.Processed != 10 {
		t.Errorf("Processed mismatch: expected 10, got %d", decoded.Processed)
	}
	if decoded.Errors != 2 {
		t.Errorf("Errors mismatch: expected 2, got %d", decoded.Errors)
	}
	if decoded.Message != "Partially processed" {
		t.Errorf("Message mismatch")
	}
}

func TestHeartbeatResponse_SuccessCase(t *testing.T) {
	resp := HeartbeatResponse{
		Success:   true,
		Processed: 5,
		Errors:    0,
	}

	if !resp.Success {
		t.Error("Expected success to be true")
	}
	if resp.Errors != 0 {
		t.Error("Expected no errors")
	}
}

func TestHeartbeatResponse_FailureCase(t *testing.T) {
	resp := HeartbeatResponse{
		Success:   false,
		Processed: 0,
		Errors:    5,
		Message:   "All heartbeats failed",
	}

	if resp.Success {
		t.Error("Expected success to be false")
	}
	if resp.Processed != 0 {
		t.Error("Expected 0 processed")
	}
}

func TestHeartbeatPayload_EmptyHeartbeats(t *testing.T) {
	payload := HeartbeatPayload{
		Heartbeats: []HeartbeatData{},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded HeartbeatPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Heartbeats) != 0 {
		t.Errorf("Expected 0 heartbeats, got %d", len(decoded.Heartbeats))
	}
}

func TestHeartbeatPayload_MultipleHeartbeats(t *testing.T) {
	payload := HeartbeatPayload{
		Heartbeats: []HeartbeatData{
			{HeartbeatID: "id-1", Entity: "file1.go", Time: 1000},
			{HeartbeatID: "id-2", Entity: "file2.go", Time: 2000},
			{HeartbeatID: "id-3", Entity: "file3.go", Time: 3000},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded HeartbeatPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Heartbeats) != 3 {
		t.Fatalf("Expected 3 heartbeats, got %d", len(decoded.Heartbeats))
	}

	for i, hb := range decoded.Heartbeats {
		expectedID := "id-" + string(rune('1'+i))
		if hb.HeartbeatID != expectedID {
			t.Errorf("Heartbeat %d: expected ID %s, got %s", i, expectedID, hb.HeartbeatID)
		}
	}
}

func TestHeartbeatData_IsWrite(t *testing.T) {
	testCases := []struct {
		name    string
		isWrite bool
	}{
		{"write event", true},
		{"read event", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hb := HeartbeatData{
				HeartbeatID: "test",
				Entity:      "file.go",
				Time:        1234567890,
				IsWrite:     tc.isWrite,
			}

			data, _ := json.Marshal(hb)
			var decoded HeartbeatData
			json.Unmarshal(data, &decoded)

			if decoded.IsWrite != tc.isWrite {
				t.Errorf("IsWrite mismatch: expected %v, got %v", tc.isWrite, decoded.IsWrite)
			}
		})
	}
}

func TestHeartbeatData_EntityTypes(t *testing.T) {
	entityTypes := []string{"file", "app", "domain"}

	for _, et := range entityTypes {
		t.Run(et, func(t *testing.T) {
			hb := HeartbeatData{
				HeartbeatID: "test",
				Entity:      "test-entity",
				EntityType:  et,
				Time:        1234567890,
			}

			data, _ := json.Marshal(hb)
			var decoded HeartbeatData
			json.Unmarshal(data, &decoded)

			if decoded.EntityType != et {
				t.Errorf("EntityType mismatch: expected %s, got %s", et, decoded.EntityType)
			}
		})
	}
}

func TestHeartbeatData_Categories(t *testing.T) {
	categories := []string{"coding", "debugging", "browsing", "building", "running_tests"}

	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			hb := HeartbeatData{
				HeartbeatID: "test",
				Entity:      "test-entity",
				Category:    cat,
				Time:        1234567890,
			}

			data, _ := json.Marshal(hb)
			var decoded HeartbeatData
			json.Unmarshal(data, &decoded)

			if decoded.Category != cat {
				t.Errorf("Category mismatch: expected %s, got %s", cat, decoded.Category)
			}
		})
	}
}
