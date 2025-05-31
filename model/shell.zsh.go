// service/zsh_hook_service.go
package model

import (
	"fmt"
	"os"
	"strings"

	"github.com/gookit/color"
)

type ZshHookService struct {
	BaseHookService
	shellName  string
	configPath string
	hookLines  []string
}

func NewZshHookService() ShellHookService {
	sourceContent := os.ExpandEnv(fmt.Sprintf("$HOME/%s/hooks/zsh.zsh", COMMAND_BASE_STORAGE_FOLDER))
	return &ZshHookService{
		shellName:  "zsh",
		configPath: os.ExpandEnv("$HOME/.zshrc"),
		hookLines: []string{
			"# Added by shelltime CLI",
			fmt.Sprintf("export PATH=\"$HOME/%s/bin:$PATH\"", COMMAND_BASE_STORAGE_FOLDER),
			fmt.Sprintf("source %s", sourceContent),
		},
	}
}

func (s *ZshHookService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *ZshHookService) ShellName() string {
	return s.shellName
}

func (s *ZshHookService) Install() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("zsh config file not found at %s. Please run 'shelltime hooks install'", s.configPath)
	}

	// Backup the file
	if err := s.backupFile(s.configPath); err != nil {
		return err
	}
	if err := s.Check(); err == nil {
		color.Green.Println("Zsh hook is already installed.")
		return nil
	}

	// Add hook lines
	return s.addHookLines(s.configPath, s.hookLines)
}

func (s *ZshHookService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	// Backup the file
	if err := s.backupFile(s.configPath); err != nil {
		return err
	}

	return s.removeHookLines(s.configPath, s.hookLines)
}

func (s *ZshHookService) Check() error {
	content, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("zsh config file not found at %s. Please run 'shelltime hooks install'", s.configPath)
		}
		return fmt.Errorf("failed to read zsh config file %s: %w", s.configPath, err)
	}

	fileContent := string(content)
	for _, hookLine := range s.hookLines {
		if !strings.Contains(fileContent, hookLine) {
			return fmt.Errorf("hook line missing in %s: '%s'. Please run 'shelltime hooks install' or manually add it", s.configPath, hookLine)
		}
	}

	return nil
}
