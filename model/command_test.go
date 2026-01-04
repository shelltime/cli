package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommand_ToLine(t *testing.T) {
	cmd := Command{
		Shell:     "bash",
		SessionID: 12345,
		Command:   "ls -la",
		Main:      "ls",
		Hostname:  "testhost",
		Username:  "testuser",
		Time:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 15, 10, 30, 5, 0, time.UTC),
		Result:    0,
		Phase:     CommandPhasePre,
	}

	recordingTime := time.Date(2024, 1, 15, 10, 30, 1, 0, time.UTC)
	line, err := cmd.ToLine(recordingTime)
	if err != nil {
		t.Fatalf("ToLine failed: %v", err)
	}

	// Line should contain JSON followed by separator and timestamp
	if !strings.Contains(string(line), string(SEPARATOR)) {
		t.Error("Line should contain separator")
	}

	// Line should end with newline
	if !strings.HasSuffix(string(line), "\n") {
		t.Error("Line should end with newline")
	}

	// Parse the JSON part
	parts := strings.Split(strings.TrimSuffix(string(line), "\n"), string(SEPARATOR))
	if len(parts) != 2 {
		t.Fatalf("Expected 2 parts, got %d", len(parts))
	}

	var parsedCmd Command
	if err := json.Unmarshal([]byte(parts[0]), &parsedCmd); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsedCmd.Shell != cmd.Shell {
		t.Errorf("Expected shell %s, got %s", cmd.Shell, parsedCmd.Shell)
	}
	if parsedCmd.Command != cmd.Command {
		t.Errorf("Expected command %s, got %s", cmd.Command, parsedCmd.Command)
	}
}

func TestCommand_FromLine(t *testing.T) {
	originalCmd := Command{
		Shell:     "zsh",
		SessionID: 67890,
		Command:   "git status",
		Main:      "git",
		Hostname:  "devbox",
		Username:  "developer",
		Time:      time.Date(2024, 2, 20, 14, 0, 0, 0, time.UTC),
		EndTime:   time.Time{},
		Result:    0,
		Phase:     CommandPhasePre,
	}

	recordingTime := time.Date(2024, 2, 20, 14, 0, 1, 0, time.UTC)
	line, err := originalCmd.ToLine(recordingTime)
	if err != nil {
		t.Fatalf("ToLine failed: %v", err)
	}

	var parsedCmd Command
	parsedRecordingTime, err := parsedCmd.FromLine(strings.TrimSuffix(string(line), "\n"))
	if err != nil {
		t.Fatalf("FromLine failed: %v", err)
	}

	// Verify all fields
	if parsedCmd.Shell != originalCmd.Shell {
		t.Errorf("Shell mismatch: expected %s, got %s", originalCmd.Shell, parsedCmd.Shell)
	}
	if parsedCmd.SessionID != originalCmd.SessionID {
		t.Errorf("SessionID mismatch: expected %d, got %d", originalCmd.SessionID, parsedCmd.SessionID)
	}
	if parsedCmd.Command != originalCmd.Command {
		t.Errorf("Command mismatch: expected %s, got %s", originalCmd.Command, parsedCmd.Command)
	}
	if parsedCmd.Username != originalCmd.Username {
		t.Errorf("Username mismatch: expected %s, got %s", originalCmd.Username, parsedCmd.Username)
	}

	// Recording time should match
	if !parsedRecordingTime.Equal(recordingTime) {
		t.Errorf("RecordingTime mismatch: expected %v, got %v", recordingTime, parsedRecordingTime)
	}
}

func TestCommand_FromLine_InvalidFormat(t *testing.T) {
	testCases := []struct {
		name string
		line string
	}{
		{"empty line", ""},
		{"no separator", `{"shell":"bash"}`},
		{"invalid json", `not json` + string(SEPARATOR) + "123456789"},
		{"invalid timestamp", `{"shell":"bash"}` + string(SEPARATOR) + "not-a-number"},
		{"too many parts", `{"shell":"bash"}` + string(SEPARATOR) + "123" + string(SEPARATOR) + "extra"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cmd Command
			_, err := cmd.FromLine(tc.line)
			if err == nil {
				t.Error("Expected error for invalid line format")
			}
		})
	}
}

func TestCommand_FromLineBytes(t *testing.T) {
	originalCmd := Command{
		Shell:     "fish",
		SessionID: 11111,
		Command:   "echo hello",
		Main:      "echo",
		Hostname:  "fishbox",
		Username:  "fishuser",
		Time:      time.Date(2024, 3, 10, 9, 0, 0, 0, time.UTC),
		Phase:     CommandPhasePost,
		Result:    0,
	}

	recordingTime := time.Date(2024, 3, 10, 9, 0, 2, 0, time.UTC)
	line, err := originalCmd.ToLine(recordingTime)
	if err != nil {
		t.Fatalf("ToLine failed: %v", err)
	}

	// Remove the trailing newline for FromLineBytes
	lineBytes := line[:len(line)-1]

	var parsedCmd Command
	parsedRecordingTime, err := parsedCmd.FromLineBytes(lineBytes)
	if err != nil {
		t.Fatalf("FromLineBytes failed: %v", err)
	}

	if parsedCmd.Shell != originalCmd.Shell {
		t.Errorf("Shell mismatch: expected %s, got %s", originalCmd.Shell, parsedCmd.Shell)
	}

	if !parsedRecordingTime.Equal(recordingTime) {
		t.Errorf("RecordingTime mismatch: expected %v, got %v", recordingTime, parsedRecordingTime)
	}
}

func TestCommand_FromLineBytes_InvalidFormat(t *testing.T) {
	testCases := []struct {
		name string
		line []byte
	}{
		{"empty line", []byte{}},
		{"no separator", []byte(`{"shell":"bash"}`)},
		{"invalid json", append([]byte("not json"), SEPARATOR, '1', '2', '3')},
		{"invalid timestamp", append([]byte(`{"shell":"bash"}`), SEPARATOR, 'x', 'y', 'z')},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cmd Command
			_, err := cmd.FromLineBytes(tc.line)
			if err == nil {
				t.Error("Expected error for invalid line format")
			}
		})
	}
}

func TestCommand_IsSame(t *testing.T) {
	cmd1 := Command{
		Shell:     "bash",
		SessionID: 12345,
		Command:   "ls -la",
		Username:  "testuser",
	}

	testCases := []struct {
		name     string
		cmd2     Command
		expected bool
	}{
		{
			"identical",
			Command{Shell: "bash", SessionID: 12345, Command: "ls -la", Username: "testuser"},
			true,
		},
		{
			"different shell",
			Command{Shell: "zsh", SessionID: 12345, Command: "ls -la", Username: "testuser"},
			false,
		},
		{
			"different session",
			Command{Shell: "bash", SessionID: 99999, Command: "ls -la", Username: "testuser"},
			false,
		},
		{
			"different command",
			Command{Shell: "bash", SessionID: 12345, Command: "pwd", Username: "testuser"},
			false,
		},
		{
			"different username",
			Command{Shell: "bash", SessionID: 12345, Command: "ls -la", Username: "otheruser"},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := cmd1.IsSame(tc.cmd2)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestCommand_IsPairPreCommand(t *testing.T) {
	now := time.Now()

	preCmd := Command{
		Shell:     "bash",
		SessionID: 12345,
		Command:   "ls -la",
		Username:  "testuser",
		Time:      now.Add(-time.Hour), // 1 hour ago
		Phase:     CommandPhasePre,
	}

	testCases := []struct {
		name     string
		target   Command
		expected bool
	}{
		{
			"matching pre command",
			Command{Shell: "bash", SessionID: 12345, Command: "ls -la", Username: "testuser"},
			true,
		},
		{
			"post phase command",
			Command{Shell: "bash", SessionID: 12345, Command: "ls -la", Username: "testuser"},
			true,
		},
		{
			"different command",
			Command{Shell: "bash", SessionID: 12345, Command: "pwd", Username: "testuser"},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := preCmd.IsPairPreCommand(tc.target)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestCommand_IsPairPreCommand_OldCommand(t *testing.T) {
	// Command older than 10 days
	oldPreCmd := Command{
		Shell:     "bash",
		SessionID: 12345,
		Command:   "ls -la",
		Username:  "testuser",
		Time:      time.Now().Add(-11 * 24 * time.Hour), // 11 days ago
		Phase:     CommandPhasePre,
	}

	target := Command{Shell: "bash", SessionID: 12345, Command: "ls -la", Username: "testuser"}

	if oldPreCmd.IsPairPreCommand(target) {
		t.Error("Should not pair with command older than 10 days")
	}
}

func TestCommand_IsPairPreCommand_NotPrePhase(t *testing.T) {
	postCmd := Command{
		Shell:     "bash",
		SessionID: 12345,
		Command:   "ls -la",
		Username:  "testuser",
		Time:      time.Now().Add(-time.Hour),
		Phase:     CommandPhasePost, // Not a pre command
	}

	target := Command{Shell: "bash", SessionID: 12345, Command: "ls -la", Username: "testuser"}

	if postCmd.IsPairPreCommand(target) {
		t.Error("Should not pair with post phase command")
	}
}

func TestCommand_IsNil(t *testing.T) {
	testCases := []struct {
		name     string
		cmd      Command
		expected bool
	}{
		{"empty command", Command{}, true},
		{"has command", Command{Command: "ls"}, false},
		{"has session id", Command{SessionID: 123}, false},
		{"has username", Command{Username: "user"}, false},
		{"has shell", Command{Shell: "bash"}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.cmd.IsNil()
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestCommand_GetUniqueKey(t *testing.T) {
	cmd := Command{
		Shell:     "bash",
		SessionID: 12345,
		Command:   "git commit -m 'test'",
		Username:  "developer",
	}

	key := cmd.GetUniqueKey()
	expected := "bash|12345|git commit -m 'test'|developer"

	if key != expected {
		t.Errorf("Expected key %s, got %s", expected, key)
	}
}

func TestCommand_FindClosestCommand(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	targetCmd := Command{
		Shell:     "bash",
		SessionID: 100,
		Command:   "ls",
		Username:  "user",
		Time:      baseTime,
	}

	commands := []*Command{
		{Shell: "bash", SessionID: 100, Command: "ls", Username: "user", Time: baseTime.Add(-10 * time.Second)},
		{Shell: "bash", SessionID: 100, Command: "ls", Username: "user", Time: baseTime.Add(-5 * time.Second)},  // Closest
		{Shell: "bash", SessionID: 100, Command: "ls", Username: "user", Time: baseTime.Add(-30 * time.Second)},
		nil, // Should be skipped
	}

	closest := targetCmd.FindClosestCommand(commands, true)
	if closest == nil {
		t.Fatal("Expected to find a closest command")
	}

	expectedTime := baseTime.Add(-5 * time.Second)
	if !closest.Time.Equal(expectedTime) {
		t.Errorf("Expected closest command time %v, got %v", expectedTime, closest.Time)
	}
}

func TestCommand_FindClosestCommand_WithoutSameKey(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	targetCmd := Command{
		Shell:     "bash",
		SessionID: 100,
		Command:   "ls",
		Username:  "user",
		Time:      baseTime,
	}

	// Commands with different keys
	commands := []*Command{
		{Shell: "zsh", SessionID: 200, Command: "pwd", Username: "other", Time: baseTime.Add(-5 * time.Second)},
		{Shell: "fish", SessionID: 300, Command: "cd", Username: "another", Time: baseTime.Add(-10 * time.Second)},
	}

	// Without same key requirement, should still find the closest
	closest := targetCmd.FindClosestCommand(commands, false)
	if closest == nil {
		t.Fatal("Expected to find a closest command")
	}

	expectedTime := baseTime.Add(-5 * time.Second)
	if !closest.Time.Equal(expectedTime) {
		t.Errorf("Expected closest command time %v, got %v", expectedTime, closest.Time)
	}
}

func TestCommand_FindClosestCommand_NoMatch(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	targetCmd := Command{
		Shell:     "bash",
		SessionID: 100,
		Command:   "ls",
		Username:  "user",
		Time:      baseTime,
	}

	// Commands with different keys and withSameKey=true
	commands := []*Command{
		{Shell: "zsh", SessionID: 200, Command: "pwd", Username: "other", Time: baseTime.Add(-5 * time.Second)},
	}

	closest := targetCmd.FindClosestCommand(commands, true)
	if !closest.IsNil() {
		t.Error("Expected no match when requiring same key")
	}
}

func TestCommand_FindClosestCommand_FutureCommands(t *testing.T) {
	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	targetCmd := Command{
		Shell:     "bash",
		SessionID: 100,
		Command:   "ls",
		Username:  "user",
		Time:      baseTime,
	}

	// All commands are in the future (positive time diff)
	commands := []*Command{
		{Shell: "bash", SessionID: 100, Command: "ls", Username: "user", Time: baseTime.Add(5 * time.Second)},
		{Shell: "bash", SessionID: 100, Command: "ls", Username: "user", Time: baseTime.Add(10 * time.Second)},
	}

	// Future commands should not match (timeDiff < 0)
	closest := targetCmd.FindClosestCommand(commands, true)
	if closest == nil {
		t.Fatal("Expected non-nil command (even if empty)")
	}
}

func TestCommand_FindClosestCommand_EmptyList(t *testing.T) {
	targetCmd := Command{
		Shell:     "bash",
		SessionID: 100,
		Command:   "ls",
		Username:  "user",
		Time:      time.Now(),
	}

	closest := targetCmd.FindClosestCommand([]*Command{}, true)
	if !closest.IsNil() {
		t.Error("Expected nil command for empty list")
	}
}

func TestCommandPhase_Constants(t *testing.T) {
	if CommandPhasePre != 0 {
		t.Errorf("Expected CommandPhasePre to be 0, got %d", CommandPhasePre)
	}
	if CommandPhasePost != 1 {
		t.Errorf("Expected CommandPhasePost to be 1, got %d", CommandPhasePost)
	}
}

func TestCommand_DoSavePre(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override the storage folder
	originalBaseFolder := COMMAND_BASE_STORAGE_FOLDER
	defer func() { COMMAND_BASE_STORAGE_FOLDER = originalBaseFolder }()

	// Use a unique base folder that will be under the temp dir
	InitFolder("test-save-pre")
	// Replace with temp dir path
	COMMAND_BASE_STORAGE_FOLDER = filepath.Join(tempDir, ".shelltime-test-save-pre")
	COMMAND_STORAGE_FOLDER = COMMAND_BASE_STORAGE_FOLDER + "/commands"
	COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"

	cmd := Command{
		Shell:     "bash",
		SessionID: 12345,
		Command:   "ls -la",
		Main:      "ls",
		Hostname:  "testhost",
		Username:  "testuser",
		Time:      time.Now(),
		Phase:     CommandPhasePre,
	}

	err = cmd.DoSavePre()
	if err != nil {
		t.Fatalf("DoSavePre failed: %v", err)
	}

	// Verify the file was created using the same path helper function
	preFilePath := GetPreCommandFilePath()
	if _, err := os.Stat(preFilePath); os.IsNotExist(err) {
		t.Error("Pre-command file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(preFilePath)
	if err != nil {
		t.Fatalf("Failed to read pre-command file: %v", err)
	}

	if !strings.Contains(string(content), "ls -la") {
		t.Error("Pre-command file should contain the command")
	}
}

func TestCommand_DoUpdate(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override the storage folder
	originalBaseFolder := COMMAND_BASE_STORAGE_FOLDER
	defer func() { COMMAND_BASE_STORAGE_FOLDER = originalBaseFolder }()

	COMMAND_BASE_STORAGE_FOLDER = filepath.Join(tempDir, ".shelltime-test-update")
	COMMAND_STORAGE_FOLDER = COMMAND_BASE_STORAGE_FOLDER + "/commands"
	COMMAND_POST_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/post.txt"

	cmd := Command{
		Shell:     "bash",
		SessionID: 12345,
		Command:   "make build",
		Main:      "make",
		Hostname:  "testhost",
		Username:  "testuser",
		Time:      time.Now(),
		Phase:     CommandPhasePre,
	}

	err = cmd.DoUpdate(0) // Exit code 0
	if err != nil {
		t.Fatalf("DoUpdate failed: %v", err)
	}

	// Verify the file was created using the same path helper function
	postFilePath := GetPostCommandFilePath()
	if _, err := os.Stat(postFilePath); os.IsNotExist(err) {
		t.Error("Post-command file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(postFilePath)
	if err != nil {
		t.Fatalf("Failed to read post-command file: %v", err)
	}

	if !strings.Contains(string(content), "make build") {
		t.Error("Post-command file should contain the command")
	}

	// Verify result code is in the content
	if !strings.Contains(string(content), `"result":0`) {
		t.Error("Post-command file should contain result code")
	}
}

func TestEnsureStorageFolder(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override the storage folder
	originalBaseFolder := COMMAND_BASE_STORAGE_FOLDER
	defer func() { COMMAND_BASE_STORAGE_FOLDER = originalBaseFolder }()

	COMMAND_BASE_STORAGE_FOLDER = filepath.Join(tempDir, ".shelltime-ensure")
	COMMAND_STORAGE_FOLDER = COMMAND_BASE_STORAGE_FOLDER + "/commands"

	err = ensureStorageFolder()
	if err != nil {
		t.Fatalf("ensureStorageFolder failed: %v", err)
	}

	// Verify folder was created using the same path helper function
	if _, err := os.Stat(GetCommandsStoragePath()); os.IsNotExist(err) {
		t.Error("Storage folder was not created")
	}

	// Call again to ensure it doesn't fail when folder exists
	err = ensureStorageFolder()
	if err != nil {
		t.Fatalf("ensureStorageFolder failed on second call: %v", err)
	}
}
