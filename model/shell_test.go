package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test BaseHookService

func TestBaseHookService_BackupFile_FileNotExists(t *testing.T) {
	b := &BaseHookService{}
	err := b.backupFile("/nonexistent/path/file.txt")
	if err != nil {
		t.Errorf("backupFile should not error for non-existent file: %v", err)
	}
}

func TestBaseHookService_BackupFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-shell-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "testrc")
	content := "# Test config\nexport PATH=$PATH:/usr/bin\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	b := &BaseHookService{}
	err = b.backupFile(testFile)
	if err != nil {
		t.Fatalf("backupFile failed: %v", err)
	}

	// Check that backup was created
	files, _ := os.ReadDir(tempDir)
	backupFound := false
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "testrc.bak.") {
			backupFound = true
			// Verify backup content
			backupContent, _ := os.ReadFile(filepath.Join(tempDir, f.Name()))
			if string(backupContent) != content {
				t.Error("Backup content doesn't match original")
			}
			break
		}
	}

	if !backupFound {
		t.Error("Backup file was not created")
	}
}

func TestBaseHookService_AddHookLines(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-shell-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "testrc")
	initialContent := "# Existing config\nexport EDITOR=vim\n"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	b := &BaseHookService{}
	hookLines := []string{
		"# Added by shelltime",
		"source ~/.shelltime/hooks/test.sh",
	}

	err = b.addHookLines(testFile, hookLines)
	if err != nil {
		t.Fatalf("addHookLines failed: %v", err)
	}

	// Verify content
	content, _ := os.ReadFile(testFile)
	for _, line := range hookLines {
		if !strings.Contains(string(content), line) {
			t.Errorf("Expected file to contain: %s", line)
		}
	}
}

func TestBaseHookService_RemoveHookLines(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-shell-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file with hook lines
	testFile := filepath.Join(tempDir, "testrc")
	initialContent := `# Existing config
export EDITOR=vim
# Added by shelltime
source ~/.shelltime/hooks/test.sh
export PATH="~/.shelltime/bin:$PATH"
`
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	b := &BaseHookService{}
	hookLines := []string{
		"# Added by shelltime",
		"source ~/.shelltime/hooks/test.sh",
	}

	err = b.removeHookLines(testFile, hookLines)
	if err != nil {
		t.Fatalf("removeHookLines failed: %v", err)
	}

	// Verify content - hook lines should be removed
	content, _ := os.ReadFile(testFile)
	for _, line := range hookLines {
		if strings.Contains(string(content), line) {
			t.Errorf("Expected file to NOT contain: %s", line)
		}
	}

	// Original content should still be there
	if !strings.Contains(string(content), "export EDITOR=vim") {
		t.Error("Original content should be preserved")
	}
}

func TestBaseHookService_RemoveHookLines_FileNotExists(t *testing.T) {
	b := &BaseHookService{}
	err := b.removeHookLines("/nonexistent/path/file.txt", []string{"line1"})
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestBaseHookService_AddHookLines_FileNotExists(t *testing.T) {
	b := &BaseHookService{}
	err := b.addHookLines("/nonexistent/path/file.txt", []string{"line1"})
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// Test BashHookService

func TestNewBashHookService(t *testing.T) {
	service := NewBashHookService()
	if service == nil {
		t.Fatal("NewBashHookService returned nil")
	}

	bashService, ok := service.(*BashHookService)
	if !ok {
		t.Fatal("NewBashHookService did not return *BashHookService")
	}

	if bashService.shellName != "bash" {
		t.Errorf("Expected shellName 'bash', got '%s'", bashService.shellName)
	}

	if !strings.HasSuffix(bashService.configPath, ".bashrc") {
		t.Errorf("Expected configPath to end with .bashrc, got '%s'", bashService.configPath)
	}
}

func TestBashHookService_Match(t *testing.T) {
	service := NewBashHookService()

	testCases := []struct {
		input    string
		expected bool
	}{
		{"bash", true},
		{"BASH", true},
		{"/bin/bash", true},
		{"/usr/bin/bash", true},
		{"zsh", false},
		{"fish", false},
		{"sh", false},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := service.Match(tc.input)
			if result != tc.expected {
				t.Errorf("Match(%q) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestBashHookService_ShellName(t *testing.T) {
	service := NewBashHookService()
	if service.ShellName() != "bash" {
		t.Errorf("Expected 'bash', got '%s'", service.ShellName())
	}
}

func TestBashHookService_Check_FileNotExists(t *testing.T) {
	// Create a service with non-existent config path
	service := &BashHookService{
		shellName:  "bash",
		configPath: "/nonexistent/path/.bashrc",
		hookLines:  []string{"# test"},
	}

	err := service.Check()
	if err == nil {
		t.Error("Expected error when config file doesn't exist")
	}
}

func TestBashHookService_Check_MissingHooks(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-shell-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a bashrc without hooks
	configPath := filepath.Join(tempDir, ".bashrc")
	if err := os.WriteFile(configPath, []byte("# Empty bashrc\n"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	service := &BashHookService{
		shellName:  "bash",
		configPath: configPath,
		hookLines:  []string{"# Added by shelltime", "source test"},
	}

	err = service.Check()
	if err == nil {
		t.Error("Expected error when hook lines are missing")
	}
}

func TestBashHookService_Check_HooksPresent(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "shelltime-shell-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hookLines := []string{"# Added by shelltime", "source test"}

	// Create a bashrc with hooks
	configPath := filepath.Join(tempDir, ".bashrc")
	content := strings.Join(hookLines, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	service := &BashHookService{
		shellName:  "bash",
		configPath: configPath,
		hookLines:  hookLines,
	}

	err = service.Check()
	if err != nil {
		t.Errorf("Expected no error when hooks are present: %v", err)
	}
}

func TestBashHookService_Uninstall_FileNotExists(t *testing.T) {
	service := &BashHookService{
		shellName:  "bash",
		configPath: "/nonexistent/path/.bashrc",
		hookLines:  []string{"# test"},
	}

	err := service.Uninstall()
	if err != nil {
		t.Errorf("Uninstall should not error when file doesn't exist: %v", err)
	}
}

// Test ZshHookService

func TestNewZshHookService(t *testing.T) {
	service := NewZshHookService()
	if service == nil {
		t.Fatal("NewZshHookService returned nil")
	}

	zshService, ok := service.(*ZshHookService)
	if !ok {
		t.Fatal("NewZshHookService did not return *ZshHookService")
	}

	if zshService.shellName != "zsh" {
		t.Errorf("Expected shellName 'zsh', got '%s'", zshService.shellName)
	}

	if !strings.HasSuffix(zshService.configPath, ".zshrc") {
		t.Errorf("Expected configPath to end with .zshrc, got '%s'", zshService.configPath)
	}
}

func TestZshHookService_Match(t *testing.T) {
	service := NewZshHookService()

	testCases := []struct {
		input    string
		expected bool
	}{
		{"zsh", true},
		{"ZSH", true},
		{"/bin/zsh", true},
		{"/usr/local/bin/zsh", true},
		{"bash", false},
		{"fish", false},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := service.Match(tc.input)
			if result != tc.expected {
				t.Errorf("Match(%q) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestZshHookService_ShellName(t *testing.T) {
	service := NewZshHookService()
	if service.ShellName() != "zsh" {
		t.Errorf("Expected 'zsh', got '%s'", service.ShellName())
	}
}

func TestZshHookService_Check_FileNotExists(t *testing.T) {
	service := &ZshHookService{
		shellName:  "zsh",
		configPath: "/nonexistent/path/.zshrc",
		hookLines:  []string{"# test"},
	}

	err := service.Check()
	if err == nil {
		t.Error("Expected error when config file doesn't exist")
	}
}

func TestZshHookService_Uninstall_FileNotExists(t *testing.T) {
	service := &ZshHookService{
		shellName:  "zsh",
		configPath: "/nonexistent/path/.zshrc",
		hookLines:  []string{"# test"},
	}

	err := service.Uninstall()
	if err != nil {
		t.Errorf("Uninstall should not error when file doesn't exist: %v", err)
	}
}

// Test FishHookService

func TestNewFishHookService(t *testing.T) {
	service := NewFishHookService()
	if service == nil {
		t.Fatal("NewFishHookService returned nil")
	}

	fishService, ok := service.(*FishHookService)
	if !ok {
		t.Fatal("NewFishHookService did not return *FishHookService")
	}

	if fishService.shellName != "fish" {
		t.Errorf("Expected shellName 'fish', got '%s'", fishService.shellName)
	}

	if !strings.Contains(fishService.configPath, "fish/config.fish") {
		t.Errorf("Expected configPath to contain fish/config.fish, got '%s'", fishService.configPath)
	}
}

func TestFishHookService_Match(t *testing.T) {
	service := NewFishHookService()

	testCases := []struct {
		input    string
		expected bool
	}{
		{"fish", true},
		{"FISH", true},
		{"/usr/bin/fish", true},
		{"/usr/local/bin/fish", true},
		{"bash", false},
		{"zsh", false},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := service.Match(tc.input)
			if result != tc.expected {
				t.Errorf("Match(%q) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestFishHookService_ShellName(t *testing.T) {
	service := NewFishHookService()
	if service.ShellName() != "fish" {
		t.Errorf("Expected 'fish', got '%s'", service.ShellName())
	}
}

func TestFishHookService_Check_FileNotExists(t *testing.T) {
	service := &FishHookService{
		shellName:  "fish",
		configPath: "/nonexistent/path/config.fish",
		hookLines:  []string{"# test"},
	}

	err := service.Check()
	if err == nil {
		t.Error("Expected error when config file doesn't exist")
	}
}

func TestFishHookService_Uninstall_FileNotExists(t *testing.T) {
	service := &FishHookService{
		shellName:  "fish",
		configPath: "/nonexistent/path/config.fish",
		hookLines:  []string{"# test"},
	}

	err := service.Uninstall()
	if err != nil {
		t.Errorf("Uninstall should not error when file doesn't exist: %v", err)
	}
}

// Interface compliance tests

func TestBashHookService_ImplementsShellHookService(t *testing.T) {
	var _ ShellHookService = &BashHookService{}
}

func TestZshHookService_ImplementsShellHookService(t *testing.T) {
	var _ ShellHookService = &ZshHookService{}
}

func TestFishHookService_ImplementsShellHookService(t *testing.T) {
	var _ ShellHookService = &FishHookService{}
}

// Test hook line content

func TestBashHookService_HookLines(t *testing.T) {
	service := NewBashHookService().(*BashHookService)

	if len(service.hookLines) == 0 {
		t.Error("Expected at least one hook line")
	}

	// First line should be a comment
	if !strings.HasPrefix(service.hookLines[0], "#") {
		t.Error("First hook line should be a comment")
	}

	// Should include PATH modification
	hasPath := false
	for _, line := range service.hookLines {
		if strings.Contains(line, "PATH") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Error("Hook lines should include PATH modification")
	}

	// Should include source command
	hasSource := false
	for _, line := range service.hookLines {
		if strings.Contains(line, "source") {
			hasSource = true
			break
		}
	}
	if !hasSource {
		t.Error("Hook lines should include source command")
	}
}

func TestZshHookService_HookLines(t *testing.T) {
	service := NewZshHookService().(*ZshHookService)

	if len(service.hookLines) == 0 {
		t.Error("Expected at least one hook line")
	}

	// Should include PATH modification
	hasPath := false
	for _, line := range service.hookLines {
		if strings.Contains(line, "PATH") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Error("Hook lines should include PATH modification")
	}
}

func TestFishHookService_HookLines(t *testing.T) {
	service := NewFishHookService().(*FishHookService)

	if len(service.hookLines) == 0 {
		t.Error("Expected at least one hook line")
	}

	// Should include fish_add_path
	hasFishPath := false
	for _, line := range service.hookLines {
		if strings.Contains(line, "fish_add_path") {
			hasFishPath = true
			break
		}
	}
	if !hasFishPath {
		t.Error("Fish hook lines should include fish_add_path")
	}
}
