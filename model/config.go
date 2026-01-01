package model

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// configFormat represents the format of a config file
type configFormat string

const (
	formatTOML configFormat = "toml"
	formatYAML configFormat = "yaml"
)

// detectFormat returns the format based on file extension
func detectFormat(filename string) configFormat {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".yaml", ".yml":
		return formatYAML
	default:
		return formatTOML
	}
}

// unmarshalConfig unmarshals config data based on format
func unmarshalConfig(data []byte, format configFormat, config *ShellTimeConfig) error {
	switch format {
	case formatYAML:
		return yaml.Unmarshal(data, config)
	default:
		return toml.Unmarshal(data, config)
	}
}

// configFiles represents discovered config files
type configFiles struct {
	baseFile    string // config.yaml, config.yml, or config.toml
	localFile   string // config.local.yaml, config.local.yml, or config.local.toml
	baseFormat  configFormat
	localFormat configFormat
}

// findConfigFiles discovers config files in priority order
// Priority: config.local.yaml > config.local.yml > config.yaml > config.yml > config.local.toml > config.toml
func findConfigFiles(configDir string) configFiles {
	result := configFiles{}

	// Check for base config files (YAML first, then TOML)
	baseFiles := []string{"config.yaml", "config.yml", "config.toml"}
	for _, name := range baseFiles {
		path := filepath.Join(configDir, name)
		if _, err := os.Stat(path); err == nil {
			result.baseFile = path
			result.baseFormat = detectFormat(name)
			break
		}
	}

	// Check for local config files (YAML first, then TOML)
	localFiles := []string{"config.local.yaml", "config.local.yml", "config.local.toml"}
	for _, name := range localFiles {
		path := filepath.Join(configDir, name)
		if _, err := os.Stat(path); err == nil {
			result.localFile = path
			result.localFormat = detectFormat(name)
			break
		}
	}

	return result
}

var UserShellTimeConfig ShellTimeConfig

type ConfigService interface {
	ReadConfigFile(ctx context.Context, opts ...ReadConfigOption) (ShellTimeConfig, error)
}

// readConfigOptions holds configuration for ReadConfigFile behavior
type readConfigOptions struct {
	skipCache bool
}

// ReadConfigOption is a functional option for ReadConfigFile
type ReadConfigOption func(*readConfigOptions)

// WithSkipCache returns an option that skips the cache and reads from disk
func WithSkipCache() ReadConfigOption {
	return func(o *readConfigOptions) {
		o.skipCache = true
	}
}

type configService struct {
	configDir    string
	cachedConfig *ShellTimeConfig
	mu           sync.RWMutex
}

// NewConfigService creates a new ConfigService that reads config files from the given directory.
// It supports both YAML (.yaml, .yml) and TOML (.toml) formats with priority:
// config.local.yaml > config.local.yml > config.yaml > config.yml > config.local.toml > config.toml
func NewConfigService(configDir string) ConfigService {
	return &configService{
		configDir: configDir,
	}
}

// mergeConfig merges local config settings into the base config
// Local settings override base settings when they are non-zero values
func mergeConfig(base, local *ShellTimeConfig) {
	if local.Token != "" {
		base.Token = local.Token
	}
	if local.APIEndpoint != "" {
		base.APIEndpoint = local.APIEndpoint
	}
	if local.WebEndpoint != "" {
		base.WebEndpoint = local.WebEndpoint
	}
	if local.FlushCount > 0 {
		base.FlushCount = local.FlushCount
	}
	if local.GCTime > 0 {
		base.GCTime = local.GCTime
	}
	if local.DataMasking != nil {
		base.DataMasking = local.DataMasking
	}
	if local.EnableMetrics != nil {
		base.EnableMetrics = local.EnableMetrics
	}
	if local.Encrypted != nil {
		base.Encrypted = local.Encrypted
	}
	if local.AI != nil {
		base.AI = local.AI
	}
	if len(local.Endpoints) > 0 {
		base.Endpoints = local.Endpoints
	}
	if len(local.Exclude) > 0 {
		base.Exclude = local.Exclude
	}
	if local.CCUsage != nil {
		base.CCUsage = local.CCUsage
	}
	// Migrate deprecated ccotel from local config
	if local.CCOtel != nil && local.AICodeOtel == nil {
		local.AICodeOtel = local.CCOtel
	}
	if local.AICodeOtel != nil {
		base.AICodeOtel = local.AICodeOtel
	}
	if local.LogCleanup != nil {
		base.LogCleanup = local.LogCleanup
	}
	if local.SocketPath != "" {
		base.SocketPath = local.SocketPath
	}
	if local.CodeTracking != nil {
		base.CodeTracking = local.CodeTracking
	}
	if local.LogCleanup != nil {
		base.LogCleanup = local.LogCleanup
	}
}

func (cs *configService) ReadConfigFile(ctx context.Context, opts ...ReadConfigOption) (config ShellTimeConfig, err error) {
	ctx, span := modelTracer.Start(ctx, "config.read")
	defer span.End()

	// Apply options
	options := &readConfigOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Check cache first (unless skipCache is set)
	if !options.skipCache {
		cs.mu.RLock()
		if cs.cachedConfig != nil {
			config = *cs.cachedConfig
			cs.mu.RUnlock()
			return config, nil
		}
		cs.mu.RUnlock()
	}

	// Discover config files with priority
	files := findConfigFiles(cs.configDir)

	slog.InfoContext(
		ctx,
		"config.ReadConfigFile discovered config files",
		slog.String("base", files.baseFile),
		slog.String("local", files.localFile),
		slog.String("base_format", string(files.baseFormat)),
		slog.String("local_format", string(files.localFormat)),
	)

	// Read base config file
	if files.baseFile == "" {
		err = fmt.Errorf("no config file found in %s", cs.configDir)
		return
	}

	existingConfig, err := os.ReadFile(files.baseFile)
	if err != nil {
		err = fmt.Errorf("failed to read config file: %w", err)
		return
	}

	err = unmarshalConfig(existingConfig, files.baseFormat, &config)
	if err != nil {
		err = fmt.Errorf("failed to parse config file: %w", err)
		return
	}

	// Migrate deprecated ccotel field to AICodeOtel (silent migration)
	if config.CCOtel != nil && config.AICodeOtel == nil {
		config.AICodeOtel = config.CCOtel
		config.CCOtel = nil
	}

	// Read and merge local config if exists
	if files.localFile != "" {
		if localConfig, localErr := os.ReadFile(files.localFile); localErr == nil {
			var localSettings ShellTimeConfig
			unmarshalErr := unmarshalConfig(localConfig, files.localFormat, &localSettings)
			if unmarshalErr != nil {
				slog.WarnContext(
					ctx,
					"failed to parse local config file",
					slog.String("file", files.localFile),
					slog.Any("err", unmarshalErr),
				)
			}
			if unmarshalErr == nil {
				mergeConfig(&config, &localSettings)
			}

		}
	}

	// default 10 and at least 3 for performance reason
	if config.FlushCount == 0 {
		config.FlushCount = 10
	}
	if config.FlushCount < 3 {
		config.FlushCount = 3
	}

	if config.GCTime == 0 {
		config.GCTime = 14
	}

	if !strings.HasPrefix(config.WebEndpoint, "http") {
		config.WebEndpoint = "https://shelltime.xyz"
	}

	truthy := true
	if config.DataMasking == nil {
		config.DataMasking = &truthy
	}

	// Initialize AI config with defaults if not present
	if config.AI == nil {
		config.AI = DefaultAIConfig
	}

	// Initialize AICodeOtel config with default port if enabled but port not set
	if config.AICodeOtel != nil && config.AICodeOtel.GRPCPort == 0 {
		config.AICodeOtel.GRPCPort = 54027 // default OTEL gRPC port
	}

	if config.AICodeOtel != nil && config.AICodeOtel.Debug != nil && *config.AICodeOtel.Debug {
		config.AICodeOtel.Debug = &truthy
	}
	if config.SocketPath == "" {
		config.SocketPath = DefaultSocketPath
	}

	// Initialize LogCleanup with defaults if not present (enabled by default with 100MB threshold)
	if config.LogCleanup == nil {
		config.LogCleanup = &LogCleanup{
			Enabled:     &truthy,
			ThresholdMB: 100,
		}
	} else {
		if config.LogCleanup.Enabled == nil {
			config.LogCleanup.Enabled = &truthy
		}
		if config.LogCleanup.ThresholdMB == 0 {
			config.LogCleanup.ThresholdMB = 100
		}
	}

	// Save to cache
	cs.mu.Lock()
	cs.cachedConfig = &config
	cs.mu.Unlock()

	UserShellTimeConfig = config
	return
}
