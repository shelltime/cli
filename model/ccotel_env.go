package model

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/gookit/color"
)

const (
	ccOtelMarkerStart = "# >>> shelltime cc otel >>>"
	ccOtelMarkerEnd   = "# <<< shelltime cc otel <<<"
	ccOtelEndpoint    = "http://localhost:54027"
)

// CCOtelEnvService interface for shell-specific env var setup
type CCOtelEnvService interface {
	Match(shellName string) bool
	Install() error
	Check() error
	Uninstall() error
	ShellName() string
}

// baseCCOtelEnvService provides common functionality
type baseCCOtelEnvService struct{}

func (b *baseCCOtelEnvService) addEnvLines(filePath string, envLines []string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Add env lines
	lines = append(lines, envLines...)

	// Write the updated content back
	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write updated file: %w", err)
	}

	return nil
}

func (b *baseCCOtelEnvService) removeEnvLines(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var newLines []string
	scanner := bufio.NewScanner(file)
	inBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ccOtelMarkerStart) {
			inBlock = true
			continue
		}
		if strings.Contains(line, ccOtelMarkerEnd) {
			inBlock = false
			continue
		}
		if !inBlock {
			newLines = append(newLines, line)
		}
	}

	// Write the filtered content back
	if err := os.WriteFile(filePath, []byte(strings.Join(newLines, "\n")+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write updated file: %w", err)
	}

	return nil
}

func (b *baseCCOtelEnvService) checkEnvLines(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	return strings.Contains(string(content), ccOtelMarkerStart), nil
}

// =============================================================================
// Bash Implementation
// =============================================================================

type BashCCOtelEnvService struct {
	baseCCOtelEnvService
	shellName  string
	configPath string
	envLines   []string
}

func NewBashCCOtelEnvService() CCOtelEnvService {
	configPath := os.ExpandEnv("$HOME/.bashrc")
	envLines := []string{
		"",
		ccOtelMarkerStart,
		"export CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"export OTEL_METRICS_EXPORTER=otlp",
		"export OTEL_LOGS_EXPORTER=otlp",
		"export OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
		"export OTEL_EXPORTER_OTLP_ENDPOINT=" + ccOtelEndpoint,
		"export OTEL_METRIC_EXPORT_INTERVAL=10000",
		"export OTEL_LOGS_EXPORT_INTERVAL=5000",
		"export OTEL_LOG_USER_PROMPTS=1",
		"export OTEL_METRICS_INCLUDE_SESSION_ID=true",
		"export OTEL_METRICS_INCLUDE_VERSION=true",
		"export OTEL_METRICS_INCLUDE_ACCOUNT_UUID=true",
		"export OTEL_RESOURCE_ATTRIBUTES=\"user.name=$(whoami),machine.name=$(hostname),team.id=shelltime,pwd=$(pwd)\"",
		ccOtelMarkerEnd,
	}

	return &BashCCOtelEnvService{
		shellName:  "bash",
		configPath: configPath,
		envLines:   envLines,
	}
}

func (s *BashCCOtelEnvService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *BashCCOtelEnvService) ShellName() string {
	return s.shellName
}

func (s *BashCCOtelEnvService) Install() error {
	// Create config file if it doesn't exist
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		if err := os.WriteFile(s.configPath, []byte(""), 0644); err != nil {
			return fmt.Errorf("failed to create bash config file: %w", err)
		}
	}

	// Remove existing env lines first
	if err := s.removeEnvLines(s.configPath); err != nil {
		return err
	}

	// Add env lines
	if err := s.addEnvLines(s.configPath, s.envLines); err != nil {
		return err
	}

	color.Green.Printf("Claude Code OTEL config installed in %s\n", s.configPath)
	return nil
}

func (s *BashCCOtelEnvService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	return s.removeEnvLines(s.configPath)
}

func (s *BashCCOtelEnvService) Check() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("bash config file not found at %s", s.configPath)
	}

	installed, err := s.checkEnvLines(s.configPath)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("Claude Code OTEL config not found in %s", s.configPath)
	}

	return nil
}

// =============================================================================
// Zsh Implementation
// =============================================================================

type ZshCCOtelEnvService struct {
	baseCCOtelEnvService
	shellName  string
	configPath string
	envLines   []string
}

func NewZshCCOtelEnvService() CCOtelEnvService {
	configPath := os.ExpandEnv("$HOME/.zshrc")
	envLines := []string{
		"",
		ccOtelMarkerStart,
		"export CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"export OTEL_METRICS_EXPORTER=otlp",
		"export OTEL_LOGS_EXPORTER=otlp",
		"export OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
		"export OTEL_EXPORTER_OTLP_ENDPOINT=" + ccOtelEndpoint,
		"export OTEL_METRIC_EXPORT_INTERVAL=10000",
		"export OTEL_LOGS_EXPORT_INTERVAL=5000",
		"export OTEL_LOG_USER_PROMPTS=1",
		"export OTEL_METRICS_INCLUDE_SESSION_ID=true",
		"export OTEL_METRICS_INCLUDE_VERSION=true",
		"export OTEL_METRICS_INCLUDE_ACCOUNT_UUID=true",
		"export OTEL_RESOURCE_ATTRIBUTES=\"user.name=$(whoami),machine.name=$(hostname),team.id=shelltime,pwd=$(pwd)\"",
		ccOtelMarkerEnd,
	}

	return &ZshCCOtelEnvService{
		shellName:  "zsh",
		configPath: configPath,
		envLines:   envLines,
	}
}

func (s *ZshCCOtelEnvService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *ZshCCOtelEnvService) ShellName() string {
	return s.shellName
}

func (s *ZshCCOtelEnvService) Install() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("zsh config file not found at %s", s.configPath)
	}

	// Remove existing env lines first
	if err := s.removeEnvLines(s.configPath); err != nil {
		return err
	}

	// Add env lines
	if err := s.addEnvLines(s.configPath, s.envLines); err != nil {
		return err
	}

	color.Green.Printf("Claude Code OTEL config installed in %s\n", s.configPath)
	return nil
}

func (s *ZshCCOtelEnvService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	return s.removeEnvLines(s.configPath)
}

func (s *ZshCCOtelEnvService) Check() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("zsh config file not found at %s", s.configPath)
	}

	installed, err := s.checkEnvLines(s.configPath)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("Claude Code OTEL config not found in %s", s.configPath)
	}

	return nil
}

// =============================================================================
// Fish Implementation
// =============================================================================

type FishCCOtelEnvService struct {
	baseCCOtelEnvService
	shellName  string
	configPath string
	envLines   []string
}

func NewFishCCOtelEnvService() CCOtelEnvService {
	configPath := os.ExpandEnv("$HOME/.config/fish/config.fish")
	envLines := []string{
		"",
		ccOtelMarkerStart,
		"set -gx CLAUDE_CODE_ENABLE_TELEMETRY 1",
		"set -gx OTEL_METRICS_EXPORTER otlp",
		"set -gx OTEL_LOGS_EXPORTER otlp",
		"set -gx OTEL_EXPORTER_OTLP_PROTOCOL grpc",
		"set -gx OTEL_EXPORTER_OTLP_ENDPOINT " + ccOtelEndpoint,
		"set -gx OTEL_METRIC_EXPORT_INTERVAL 10000",
		"set -gx OTEL_LOGS_EXPORT_INTERVAL 5000",
		"set -gx OTEL_LOG_USER_PROMPTS 1",
		"set -gx OTEL_METRICS_INCLUDE_SESSION_ID true",
		"set -gx OTEL_METRICS_INCLUDE_VERSION true",
		"set -gx OTEL_METRICS_INCLUDE_ACCOUNT_UUID true",
		"set -gx OTEL_RESOURCE_ATTRIBUTES \"user.name=$(whoami),machine.name=$(hostname),team.id=shelltime,pwd=$(pwd)\"",
		ccOtelMarkerEnd,
	}

	return &FishCCOtelEnvService{
		shellName:  "fish",
		configPath: configPath,
		envLines:   envLines,
	}
}

func (s *FishCCOtelEnvService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *FishCCOtelEnvService) ShellName() string {
	return s.shellName
}

func (s *FishCCOtelEnvService) Install() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("fish config file not found at %s", s.configPath)
	}

	// Remove existing env lines first
	if err := s.removeEnvLines(s.configPath); err != nil {
		return err
	}

	// Add env lines
	if err := s.addEnvLines(s.configPath, s.envLines); err != nil {
		return err
	}

	color.Green.Printf("Claude Code OTEL config installed in %s\n", s.configPath)
	return nil
}

func (s *FishCCOtelEnvService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	return s.removeEnvLines(s.configPath)
}

func (s *FishCCOtelEnvService) Check() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("fish config file not found at %s", s.configPath)
	}

	installed, err := s.checkEnvLines(s.configPath)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("Claude Code OTEL config not found in %s", s.configPath)
	}

	return nil
}
