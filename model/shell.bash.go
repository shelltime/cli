package model

import (
	"fmt"
	"os"
	"strings"

	"github.com/gookit/color"
)

type BashHookService struct {
	BaseHookService
	shellName  string
	configPath string
	hookLines  []string
}

func NewBashHookService() ShellHookService {
	sourceContent := os.ExpandEnv(fmt.Sprintf("$HOME/%s/hooks/bash.bash", COMMAND_BASE_STORAGE_FOLDER))
	return &BashHookService{
		shellName:  "bash",
		configPath: os.ExpandEnv("$HOME/.bashrc"), // or .bash_profile on macOS for login shells
		hookLines: []string{
			"# Added by shelltime CLI",
			fmt.Sprintf("export PATH=\"$HOME/%s/bin:$PATH\"", COMMAND_BASE_STORAGE_FOLDER),
			fmt.Sprintf("source %s", sourceContent),
		},
	}
}

func (s *BashHookService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *BashHookService) ShellName() string {
	return s.shellName
}

func (s *BashHookService) Install() error {
	hookFilePath := os.ExpandEnv(fmt.Sprintf("$HOME/%s/hooks/bash.bash", COMMAND_BASE_STORAGE_FOLDER))
	if _, err := os.Stat(hookFilePath); os.IsNotExist(err) {
		color.Red.Println("hook file not found at", hookFilePath)
		color.Red.Println("Please run 'curl -sSL https://shelltime.xyz/i | bash' first")
		return err
	}

	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		// Attempt to create .bashrc if it doesn't exist, as it's common for it not to be present by default on some systems
		file, createErr := os.Create(s.configPath)
		if createErr != nil {
			return fmt.Errorf("bash config file not found at %s and could not be created: %w. Please run 'shelltime hooks install' after creating it", s.configPath, createErr)
		}
		file.Close() // Close the newly created file
		color.Yellow.Printf("Bash config file not found at %s, created an empty one.\n", s.configPath)
	} else if err != nil {
		return fmt.Errorf("error checking bash config file %s: %w", s.configPath, err)
	}

	// Backup the file
	if err := s.backupFile(s.configPath); err != nil {
		return err
	}
	if err := s.Check(); err == nil {
		color.Green.Println("Bash hook is already installed.")
		return nil
	}

	// Add hook lines
	return s.addHookLines(s.configPath, s.hookLines)
}

func (s *BashHookService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil // If config doesn't exist, nothing to uninstall
	}

	// Backup the file
	if err := s.backupFile(s.configPath); err != nil {
		return err
	}

	return s.removeHookLines(s.configPath, s.hookLines)
}

func (s *BashHookService) Check() error {
	content, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bash config file not found at %s. Please run 'shelltime hooks install'", s.configPath)
		}
		return fmt.Errorf("failed to read bash config file %s: %w", s.configPath, err)
	}

	fileContent := string(content)
	for _, hookLine := range s.hookLines {
		if !strings.Contains(fileContent, hookLine) {
			return fmt.Errorf("hook line missing in %s: '%s'. Please run 'shelltime hooks install' or manually add it", s.configPath, hookLine)
		}
	}

	return nil
}
