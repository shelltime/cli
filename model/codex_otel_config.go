package model

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	codexConfigDir  = ".codex"
	codexConfigFile = "config.toml"
)

// CodexOtelConfigService handles Codex OTEL configuration
type CodexOtelConfigService interface {
	Install() error
	Uninstall() error
	Check() (bool, error)
}

type codexOtelConfigService struct {
	configPath string
}

// NewCodexOtelConfigService creates a new Codex OTEL config service
func NewCodexOtelConfigService() CodexOtelConfigService {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, codexConfigDir, codexConfigFile)
	return &codexOtelConfigService{
		configPath: configPath,
	}
}

// Install adds OTEL configuration to ~/.codex/config.toml
func (s *codexOtelConfigService) Install() error {
	// Ensure directory exists
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Read existing config or create empty map
	config := make(map[string]interface{})
	if data, err := os.ReadFile(s.configPath); err == nil && len(data) > 0 {
		if err := toml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse existing config: %w", err)
		}
	}

	// Add OTEL configuration
	// Format: exporter = { otlp-grpc = {endpoint = "..."} }
	config["otel"] = map[string]interface{}{
		"log_user_prompt": true,
		"exporter": map[string]interface{}{
			"otlp-grpc": map[string]interface{}{
				"endpoint": aiCodeOtelEndpoint,
			},
		},
	}

	// Write config back
	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Uninstall removes OTEL configuration from ~/.codex/config.toml
func (s *codexOtelConfigService) Uninstall() error {
	// Check if config file exists
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil // Nothing to uninstall
	}

	// Read existing config
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	config := make(map[string]interface{})
	if len(data) > 0 {
		if err := toml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}

	// Remove OTEL configuration
	delete(config, "otel")

	// Write config back
	newData, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.configPath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Check returns true if OTEL is configured in ~/.codex/config.toml
func (s *codexOtelConfigService) Check() (bool, error) {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return false, nil
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return false, fmt.Errorf("failed to read config file: %w", err)
	}

	config := make(map[string]interface{})
	if len(data) > 0 {
		if err := toml.Unmarshal(data, &config); err != nil {
			return false, fmt.Errorf("failed to parse config: %w", err)
		}
	}

	_, exists := config["otel"]
	return exists, nil
}
