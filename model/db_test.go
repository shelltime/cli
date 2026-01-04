package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitFolder(t *testing.T) {
	// Save original values
	origBase := COMMAND_BASE_STORAGE_FOLDER
	origStorage := COMMAND_STORAGE_FOLDER
	origPre := COMMAND_PRE_STORAGE_FILE
	origPost := COMMAND_POST_STORAGE_FILE
	origCursor := COMMAND_CURSOR_STORAGE_FILE
	origHeartbeat := HEARTBEAT_LOG_FILE
	origPending := SYNC_PENDING_FILE

	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origStorage
		COMMAND_PRE_STORAGE_FILE = origPre
		COMMAND_POST_STORAGE_FILE = origPost
		COMMAND_CURSOR_STORAGE_FILE = origCursor
		HEARTBEAT_LOG_FILE = origHeartbeat
		SYNC_PENDING_FILE = origPending
	}()

	// Test with empty baseFolder (should keep default)
	InitFolder("")
	if COMMAND_BASE_STORAGE_FOLDER != ".shelltime" {
		t.Errorf("Expected default base folder, got %s", COMMAND_BASE_STORAGE_FOLDER)
	}

	// Test with custom baseFolder
	InitFolder("custom")
	if COMMAND_BASE_STORAGE_FOLDER != ".shelltime-custom" {
		t.Errorf("Expected .shelltime-custom, got %s", COMMAND_BASE_STORAGE_FOLDER)
	}
	if COMMAND_STORAGE_FOLDER != ".shelltime-custom/commands" {
		t.Errorf("Expected .shelltime-custom/commands, got %s", COMMAND_STORAGE_FOLDER)
	}
	if COMMAND_PRE_STORAGE_FILE != ".shelltime-custom/commands/pre.txt" {
		t.Errorf("Expected .shelltime-custom/commands/pre.txt, got %s", COMMAND_PRE_STORAGE_FILE)
	}
}

func TestGetPreCommandsTree(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"

	// Create the commands directory
	if err := os.MkdirAll(COMMAND_STORAGE_FOLDER, 0755); err != nil {
		t.Fatalf("Failed to create commands dir: %v", err)
	}

	// Create test pre commands
	cmd1 := Command{
		Shell:     "bash",
		SessionID: 100,
		Command:   "ls -la",
		Username:  "user1",
		Time:      time.Now(),
		Phase:     CommandPhasePre,
	}
	cmd2 := Command{
		Shell:     "bash",
		SessionID: 100,
		Command:   "ls -la",
		Username:  "user1",
		Time:      time.Now().Add(-time.Hour),
		Phase:     CommandPhasePre,
	}
	cmd3 := Command{
		Shell:     "zsh",
		SessionID: 200,
		Command:   "pwd",
		Username:  "user2",
		Time:      time.Now(),
		Phase:     CommandPhasePre,
	}

	// Write commands to file
	f, err := os.Create(GetPreCommandFilePath())
	if err != nil {
		t.Fatalf("Failed to create pre file: %v", err)
	}

	for _, cmd := range []Command{cmd1, cmd2, cmd3} {
		line, _ := cmd.ToLine(time.Now())
		f.Write(line)
	}
	f.Close()

	// Test GetPreCommandsTree
	ctx := context.Background()
	tree, err := GetPreCommandsTree(ctx)
	if err != nil {
		t.Fatalf("GetPreCommandsTree failed: %v", err)
	}

	// cmd1 and cmd2 should share the same key
	key1 := cmd1.GetUniqueKey()
	if len(tree[key1]) != 2 {
		t.Errorf("Expected 2 commands for key %s, got %d", key1, len(tree[key1]))
	}

	// cmd3 should have its own key
	key3 := cmd3.GetUniqueKey()
	if len(tree[key3]) != 1 {
		t.Errorf("Expected 1 command for key %s, got %d", key3, len(tree[key3]))
	}
}

func TestGetPreCommandsTree_FileNotExists(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"

	ctx := context.Background()
	_, err = GetPreCommandsTree(ctx)
	if err == nil {
		t.Error("Expected error when file doesn't exist")
	}
}

func TestGetPreCommands(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"

	// Create the commands directory
	if err := os.MkdirAll(COMMAND_STORAGE_FOLDER, 0755); err != nil {
		t.Fatalf("Failed to create commands dir: %v", err)
	}

	// Create test pre commands
	commands := []Command{
		{Shell: "bash", SessionID: 100, Command: "ls", Username: "user", Time: time.Now(), Phase: CommandPhasePre},
		{Shell: "bash", SessionID: 101, Command: "pwd", Username: "user", Time: time.Now(), Phase: CommandPhasePre},
		{Shell: "zsh", SessionID: 102, Command: "cd", Username: "user", Time: time.Now(), Phase: CommandPhasePre},
	}

	// Write commands to file
	f, err := os.Create(GetPreCommandFilePath())
	if err != nil {
		t.Fatalf("Failed to create pre file: %v", err)
	}

	for _, cmd := range commands {
		line, _ := cmd.ToLine(time.Now())
		f.Write(line)
	}
	f.Close()

	// Test GetPreCommands
	ctx := context.Background()
	result, err := GetPreCommands(ctx)
	if err != nil {
		t.Fatalf("GetPreCommands failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 commands, got %d", len(result))
	}
}

func TestGetPreCommands_EmptyLines(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_PRE_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/pre.txt"

	// Create the commands directory
	if err := os.MkdirAll(COMMAND_STORAGE_FOLDER, 0755); err != nil {
		t.Fatalf("Failed to create commands dir: %v", err)
	}

	// Create file with empty lines
	cmd := Command{Shell: "bash", SessionID: 100, Command: "ls", Username: "user", Time: time.Now()}
	f, _ := os.Create(GetPreCommandFilePath())
	f.WriteString("\n") // Empty line
	line, _ := cmd.ToLine(time.Now())
	f.Write(line)
	f.WriteString("\n") // Empty line
	f.Close()

	ctx := context.Background()
	result, err := GetPreCommands(ctx)
	if err != nil {
		t.Fatalf("GetPreCommands failed: %v", err)
	}

	// Should only have 1 command (empty lines skipped)
	if len(result) != 1 {
		t.Errorf("Expected 1 command, got %d", len(result))
	}
}

func TestGetLastCursor_NoCursorFile(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"

	ctx := context.Background()
	cursorTime, noCursorExist, err := GetLastCursor(ctx)
	if err != nil {
		t.Fatalf("GetLastCursor failed: %v", err)
	}

	if !noCursorExist {
		t.Error("Expected noCursorExist to be true")
	}

	if !cursorTime.IsZero() {
		t.Error("Expected zero time when no cursor exists")
	}
}

func TestGetLastCursor_WithCursor(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"

	// Create the commands directory
	if err := os.MkdirAll(COMMAND_STORAGE_FOLDER, 0755); err != nil {
		t.Fatalf("Failed to create commands dir: %v", err)
	}

	// Write cursor timestamp
	expectedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cursorFile := filepath.Join(COMMAND_STORAGE_FOLDER, "cursor.txt")
	if err := os.WriteFile(cursorFile, []byte(fmt.Sprintf("%d", expectedTime.UnixNano())), 0644); err != nil {
		t.Fatalf("Failed to write cursor file: %v", err)
	}

	ctx := context.Background()
	cursorTime, noCursorExist, err := GetLastCursor(ctx)
	if err != nil {
		t.Fatalf("GetLastCursor failed: %v", err)
	}

	if noCursorExist {
		t.Error("Expected noCursorExist to be false")
	}

	if !cursorTime.Equal(expectedTime) {
		t.Errorf("Expected cursor time %v, got %v", expectedTime, cursorTime)
	}
}

func TestGetLastCursor_MultipleLines(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"

	// Create the commands directory
	if err := os.MkdirAll(COMMAND_STORAGE_FOLDER, 0755); err != nil {
		t.Fatalf("Failed to create commands dir: %v", err)
	}

	// Write multiple cursor timestamps (should use last one)
	time1 := time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC)
	time2 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC) // Last line
	cursorFile := filepath.Join(COMMAND_STORAGE_FOLDER, "cursor.txt")
	content := fmt.Sprintf("%d\n%d\n", time1.UnixNano(), time2.UnixNano())
	if err := os.WriteFile(cursorFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write cursor file: %v", err)
	}

	ctx := context.Background()
	cursorTime, _, err := GetLastCursor(ctx)
	if err != nil {
		t.Fatalf("GetLastCursor failed: %v", err)
	}

	if !cursorTime.Equal(time2) {
		t.Errorf("Expected cursor time %v (last line), got %v", time2, cursorTime)
	}
}

func TestGetLastCursor_InvalidContent(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_CURSOR_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/cursor.txt"

	// Create the commands directory
	if err := os.MkdirAll(COMMAND_STORAGE_FOLDER, 0755); err != nil {
		t.Fatalf("Failed to create commands dir: %v", err)
	}

	// Write invalid cursor content
	cursorFile := filepath.Join(COMMAND_STORAGE_FOLDER, "cursor.txt")
	if err := os.WriteFile(cursorFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("Failed to write cursor file: %v", err)
	}

	ctx := context.Background()
	_, _, err = GetLastCursor(ctx)
	if err == nil {
		t.Error("Expected error for invalid cursor content")
	}
}

func TestGetPostCommands(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_POST_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/post.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_POST_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/post.txt"

	// Create the commands directory
	if err := os.MkdirAll(COMMAND_STORAGE_FOLDER, 0755); err != nil {
		t.Fatalf("Failed to create commands dir: %v", err)
	}

	// Create test post commands
	commands := []Command{
		{Shell: "bash", SessionID: 100, Command: "make build", Username: "user", Time: time.Now(), Phase: CommandPhasePost, Result: 0},
		{Shell: "bash", SessionID: 101, Command: "make test", Username: "user", Time: time.Now(), Phase: CommandPhasePost, Result: 1},
	}

	// Write commands to file
	f, err := os.Create(GetPostCommandFilePath())
	if err != nil {
		t.Fatalf("Failed to create post file: %v", err)
	}

	for _, cmd := range commands {
		line, _ := cmd.ToLine(time.Now())
		f.Write(line)
	}
	f.Close()

	// Test GetPostCommands
	ctx := context.Background()
	content, lineCount, err := GetPostCommands(ctx)
	if err != nil {
		t.Fatalf("GetPostCommands failed: %v", err)
	}

	if lineCount != 2 {
		t.Errorf("Expected 2 lines, got %d", lineCount)
	}

	if len(content) != 2 {
		t.Errorf("Expected 2 content entries, got %d", len(content))
	}
}

func TestGetPostCommands_EmptyLines(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_POST_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/post.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_POST_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/post.txt"

	// Create the commands directory
	if err := os.MkdirAll(COMMAND_STORAGE_FOLDER, 0755); err != nil {
		t.Fatalf("Failed to create commands dir: %v", err)
	}

	// Create file with empty lines
	cmd := Command{Shell: "bash", SessionID: 100, Command: "ls", Username: "user", Time: time.Now()}
	f, _ := os.Create(GetPostCommandFilePath())
	f.WriteString("\n") // Empty line
	line, _ := cmd.ToLine(time.Now())
	f.Write(line)
	f.WriteString("\n\n") // Multiple empty lines
	f.Close()

	ctx := context.Background()
	content, lineCount, err := GetPostCommands(ctx)
	if err != nil {
		t.Fatalf("GetPostCommands failed: %v", err)
	}

	// Should only have 1 command (empty lines skipped)
	if lineCount != 1 {
		t.Errorf("Expected 1 line, got %d", lineCount)
	}

	if len(content) != 1 {
		t.Errorf("Expected 1 content entry, got %d", len(content))
	}
}

func TestGetPostCommands_FileNotExists(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "shelltime-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save and restore original paths
	origBase := COMMAND_BASE_STORAGE_FOLDER
	defer func() {
		COMMAND_BASE_STORAGE_FOLDER = origBase
		COMMAND_STORAGE_FOLDER = origBase + "/commands"
		COMMAND_POST_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/post.txt"
	}()

	COMMAND_BASE_STORAGE_FOLDER = tempDir
	COMMAND_STORAGE_FOLDER = tempDir + "/commands"
	COMMAND_POST_STORAGE_FILE = COMMAND_STORAGE_FOLDER + "/post.txt"

	ctx := context.Background()
	_, _, err = GetPostCommands(ctx)
	if err == nil {
		t.Error("Expected error when file doesn't exist")
	}
}

func TestSEPARATOR_Constant(t *testing.T) {
	if SEPARATOR != byte('\t') {
		t.Errorf("Expected SEPARATOR to be tab, got %q", SEPARATOR)
	}
}
