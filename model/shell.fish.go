// service/fish_hook_service.go
package model

import (
	"fmt"
	"os"
	"strings"

	"github.com/gookit/color"
)

type FishHookService struct {
	BaseHookService

	shellName  string
	configPath string
	hookLines  []string
}

func NewFishHookService() ShellHookService {
	sourceContent := os.ExpandEnv(fmt.Sprintf("$HOME/%s/hooks/fish.fish", COMMAND_BASE_STORAGE_FOLDER))
	hookLines := []string{
		"# Added by shelltime CLI",
		fmt.Sprintf("fish_add_path $HOME/%s/bin", COMMAND_BASE_STORAGE_FOLDER),
		fmt.Sprintf("source %s", sourceContent),
	}
	configPath := os.ExpandEnv("$HOME/.config/fish/config.fish")

	return &FishHookService{
		shellName:  "fish",
		configPath: configPath,
		hookLines:  hookLines,
	}
}

func (s *FishHookService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *FishHookService) ShellName() string {
	return s.shellName
}

func (s *FishHookService) Install() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("fish config file not found at %s. Please run 'shelltime hooks install'", s.configPath)
	}

	if err := s.Check(); err == nil {
		color.Green.Println("Fish hook is already installed.")
		return nil
	}

	// Backup the file
	if err := s.backupFile(s.configPath); err != nil {
		return err
	}

	// Add hook lines
	return s.addHookLines(s.configPath, s.hookLines)
}

func (s *FishHookService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	// Backup the file
	if err := s.backupFile(s.configPath); err != nil {
		return err
	}
	return s.removeHookLines(s.configPath, s.hookLines)
}

func (s *FishHookService) Check() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("fish config file not found at %s. Please run 'shelltime hooks install'", s.configPath)
	}

	content, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read fish config file %s: %w", s.configPath, err)
	}

	fileContent := string(content)
	for _, hookLine := range s.hookLines {
		if !strings.Contains(fileContent, hookLine) {
			return fmt.Errorf("hook line missing in %s: '%s'. Please run 'shelltime hooks install' or manually add it", s.configPath, hookLine)
		}
	}

	return nil
}
