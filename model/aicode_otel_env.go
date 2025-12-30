package model

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/gookit/color"
)

const (
	aiCodeOtelMarkerStart = "# >>> shelltime cc otel >>>"
	aiCodeOtelMarkerEnd   = "# <<< shelltime cc otel <<<"
	aiCodeOtelEndpoint    = "http://localhost:54027"
)

// AICodeOtelEnvService interface for shell-specific env var setup
type AICodeOtelEnvService interface {
	Match(shellName string) bool
	Install() error
	Check() error
	Uninstall() error
	ShellName() string
}

// baseAICodeOtelEnvService provides common functionality
type baseAICodeOtelEnvService struct{}

func (b *baseAICodeOtelEnvService) addEnvLines(filePath string, envLines []string) error {
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

func (b *baseAICodeOtelEnvService) removeEnvLines(filePath string) error {
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
		if strings.Contains(line, aiCodeOtelMarkerStart) {
			inBlock = true
			continue
		}
		if strings.Contains(line, aiCodeOtelMarkerEnd) {
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

func (b *baseAICodeOtelEnvService) checkEnvLines(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	return strings.Contains(string(content), aiCodeOtelMarkerStart), nil
}

// =============================================================================
// Bash Implementation
// =============================================================================

type BashAICodeOtelEnvService struct {
	baseAICodeOtelEnvService
	shellName  string
	configPath string
	envLines   []string
}

func NewBashAICodeOtelEnvService() AICodeOtelEnvService {
	configPath := os.ExpandEnv("$HOME/.bashrc")
	envLines := []string{
		"",
		aiCodeOtelMarkerStart,
		"export CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"export OTEL_METRICS_EXPORTER=otlp",
		"export OTEL_LOGS_EXPORTER=otlp",
		"export OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
		"export OTEL_EXPORTER_OTLP_ENDPOINT=" + aiCodeOtelEndpoint,
		"export OTEL_METRIC_EXPORT_INTERVAL=10000",
		"export OTEL_LOGS_EXPORT_INTERVAL=5000",
		"export OTEL_LOG_USER_PROMPTS=1",
		"export OTEL_METRICS_INCLUDE_SESSION_ID=true",
		"export OTEL_METRICS_INCLUDE_VERSION=true",
		"export OTEL_METRICS_INCLUDE_ACCOUNT_UUID=true",
		"export OTEL_RESOURCE_ATTRIBUTES=\"user.name=$(whoami),machine.name=$(hostname),team.id=shelltime,pwd=$(pwd)\"",
		aiCodeOtelMarkerEnd,
	}

	return &BashAICodeOtelEnvService{
		shellName:  "bash",
		configPath: configPath,
		envLines:   envLines,
	}
}

func (s *BashAICodeOtelEnvService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *BashAICodeOtelEnvService) ShellName() string {
	return s.shellName
}

func (s *BashAICodeOtelEnvService) Install() error {
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

func (s *BashAICodeOtelEnvService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	return s.removeEnvLines(s.configPath)
}

func (s *BashAICodeOtelEnvService) Check() error {
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

type ZshAICodeOtelEnvService struct {
	baseAICodeOtelEnvService
	shellName  string
	configPath string
	envLines   []string
}

func NewZshAICodeOtelEnvService() AICodeOtelEnvService {
	configPath := os.ExpandEnv("$HOME/.zshrc")
	envLines := []string{
		"",
		aiCodeOtelMarkerStart,
		"export CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"export OTEL_METRICS_EXPORTER=otlp",
		"export OTEL_LOGS_EXPORTER=otlp",
		"export OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
		"export OTEL_EXPORTER_OTLP_ENDPOINT=" + aiCodeOtelEndpoint,
		"export OTEL_METRIC_EXPORT_INTERVAL=10000",
		"export OTEL_LOGS_EXPORT_INTERVAL=5000",
		"export OTEL_LOG_USER_PROMPTS=1",
		"export OTEL_METRICS_INCLUDE_SESSION_ID=true",
		"export OTEL_METRICS_INCLUDE_VERSION=true",
		"export OTEL_METRICS_INCLUDE_ACCOUNT_UUID=true",
		"export OTEL_RESOURCE_ATTRIBUTES=\"user.name=$(whoami),machine.name=$(hostname),team.id=shelltime,pwd=$(pwd)\"",
		aiCodeOtelMarkerEnd,
	}

	return &ZshAICodeOtelEnvService{
		shellName:  "zsh",
		configPath: configPath,
		envLines:   envLines,
	}
}

func (s *ZshAICodeOtelEnvService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *ZshAICodeOtelEnvService) ShellName() string {
	return s.shellName
}

func (s *ZshAICodeOtelEnvService) Install() error {
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

func (s *ZshAICodeOtelEnvService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	return s.removeEnvLines(s.configPath)
}

func (s *ZshAICodeOtelEnvService) Check() error {
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

type FishAICodeOtelEnvService struct {
	baseAICodeOtelEnvService
	shellName  string
	configPath string
	envLines   []string
}

func NewFishAICodeOtelEnvService() AICodeOtelEnvService {
	configPath := os.ExpandEnv("$HOME/.config/fish/config.fish")
	envLines := []string{
		"",
		aiCodeOtelMarkerStart,
		"set -gx CLAUDE_CODE_ENABLE_TELEMETRY 1",
		"set -gx OTEL_METRICS_EXPORTER otlp",
		"set -gx OTEL_LOGS_EXPORTER otlp",
		"set -gx OTEL_EXPORTER_OTLP_PROTOCOL grpc",
		"set -gx OTEL_EXPORTER_OTLP_ENDPOINT " + aiCodeOtelEndpoint,
		"set -gx OTEL_METRIC_EXPORT_INTERVAL 10000",
		"set -gx OTEL_LOGS_EXPORT_INTERVAL 5000",
		"set -gx OTEL_LOG_USER_PROMPTS 1",
		"set -gx OTEL_METRICS_INCLUDE_SESSION_ID true",
		"set -gx OTEL_METRICS_INCLUDE_VERSION true",
		"set -gx OTEL_METRICS_INCLUDE_ACCOUNT_UUID true",
		"set -gx OTEL_RESOURCE_ATTRIBUTES \"user.name=$(whoami),machine.name=$(hostname),team.id=shelltime,pwd=$(pwd)\"",
		aiCodeOtelMarkerEnd,
	}

	return &FishAICodeOtelEnvService{
		shellName:  "fish",
		configPath: configPath,
		envLines:   envLines,
	}
}

func (s *FishAICodeOtelEnvService) Match(shellName string) bool {
	return strings.Contains(strings.ToLower(shellName), strings.ToLower(s.shellName))
}

func (s *FishAICodeOtelEnvService) ShellName() string {
	return s.shellName
}

func (s *FishAICodeOtelEnvService) Install() error {
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

func (s *FishAICodeOtelEnvService) Uninstall() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil
	}

	return s.removeEnvLines(s.configPath)
}

func (s *FishAICodeOtelEnvService) Check() error {
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
