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

// codexOtelManagedFlags are the [otel] flags `shelltime codex install` turns on. Together with
// the exporter they are the only keys Install writes and Uninstall removes.
var codexOtelManagedFlags = []string{
	"log_user_prompt",
	// Emits codex.agent_response with the final answer text.
	"log_agent_responses",
}

// CodexOtelConfigService handles Codex OTEL configuration
type CodexOtelConfigService interface {
	Install() error
	Uninstall() error
	Check() (bool, error)
	// Endpoint returns otel.exporter.otlp-grpc.endpoint, or "" when it is not set.
	Endpoint() (string, error)
	// MissingManagedKeys returns the managed [otel] flags that aren't set, such as
	// log_agent_responses in a config written by an older `shelltime codex install`.
	MissingManagedKeys() ([]string, error)
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

// readConfig parses ~/.codex/config.toml. A missing or empty file yields an empty map.
func (s *codexOtelConfigService) readConfig() (map[string]interface{}, error) {
	config := make(map[string]interface{})
	data, err := os.ReadFile(s.configPath)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if len(data) > 0 {
		if err := toml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
	}
	return config, nil
}

func (s *codexOtelConfigService) writeConfig(config map[string]interface{}) error {
	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// Install merges ShellTime's OTEL settings into the [otel] table of ~/.codex/config.toml,
// keeping the user's other [otel] keys (environment, trace_exporter, ...).
func (s *codexOtelConfigService) Install() error {
	// Ensure directory exists
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	config, err := s.readConfig()
	if err != nil {
		return fmt.Errorf("failed to parse existing config: %w", err)
	}

	otel, _ := config["otel"].(map[string]interface{})
	if otel == nil {
		otel = make(map[string]interface{})
	}
	for _, flag := range codexOtelManagedFlags {
		otel[flag] = true
	}
	// `exporter` selects exactly one exporter ("none", "statsig", {otlp-http = ...} or
	// {otlp-grpc = ...}), so it is replaced rather than merged.
	// Format: exporter = { otlp-grpc = {endpoint = "..."} }
	otel["exporter"] = map[string]interface{}{
		"otlp-grpc": map[string]interface{}{
			"endpoint": AICodeOtelEndpoint,
		},
	}
	config["otel"] = otel

	return s.writeConfig(config)
}

// Uninstall removes ShellTime's OTEL settings from ~/.codex/config.toml: the managed flags and
// the exporter when it still points at the ShellTime daemon. The [otel] table is deleted only
// when nothing else is left in it.
func (s *codexOtelConfigService) Uninstall() error {
	// Check if config file exists
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return nil // Nothing to uninstall
	}

	config, err := s.readConfig()
	if err != nil {
		return err
	}

	otel, ok := config["otel"].(map[string]interface{})
	if !ok {
		return nil
	}
	for _, flag := range codexOtelManagedFlags {
		delete(otel, flag)
	}
	if codexOtelGRPCEndpoint(otel) == AICodeOtelEndpoint {
		delete(otel, "exporter")
	}
	if len(otel) == 0 {
		delete(config, "otel")
	} else {
		config["otel"] = otel
	}

	return s.writeConfig(config)
}

// Check returns true if OTEL is configured in ~/.codex/config.toml
func (s *codexOtelConfigService) Check() (bool, error) {
	config, err := s.readConfig()
	if err != nil {
		return false, err
	}
	_, exists := config["otel"]
	return exists, nil
}

func (s *codexOtelConfigService) Endpoint() (string, error) {
	config, err := s.readConfig()
	if err != nil {
		return "", err
	}
	otel, _ := config["otel"].(map[string]interface{})
	return codexOtelGRPCEndpoint(otel), nil
}

func (s *codexOtelConfigService) MissingManagedKeys() ([]string, error) {
	config, err := s.readConfig()
	if err != nil {
		return nil, err
	}
	otel, _ := config["otel"].(map[string]interface{})
	var missing []string
	for _, flag := range codexOtelManagedFlags {
		if _, ok := otel[flag]; !ok {
			missing = append(missing, flag)
		}
	}
	return missing, nil
}

// codexOtelGRPCEndpoint returns exporter.otlp-grpc.endpoint of an [otel] table, or "". It walks
// generic maps because `exporter` may also be a plain string such as "none".
func codexOtelGRPCEndpoint(otel map[string]interface{}) string {
	var node interface{} = otel
	for _, key := range []string{"exporter", "otlp-grpc", "endpoint"} {
		table, ok := node.(map[string]interface{})
		if !ok {
			return ""
		}
		node = table[key]
	}
	endpoint, _ := node.(string)
	return endpoint
}
