package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

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
	configFilePath string
	cachedConfig   *ShellTimeConfig
	mu             sync.RWMutex
}

func NewConfigService(configFilePath string) ConfigService {
	return &configService{
		configFilePath: configFilePath,
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
	if local.CCOtel != nil {
		base.CCOtel = local.CCOtel
	}
	if local.LogCleanup != nil {
		base.LogCleanup = local.LogCleanup
	}
	if local.SocketPath != "" {
		base.SocketPath = local.SocketPath
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

	configFile := cs.configFilePath
	existingConfig, err := os.ReadFile(configFile)
	if err != nil {
		err = fmt.Errorf("failed to read config file: %w", err)
		return
	}

	err = toml.Unmarshal(existingConfig, &config)
	if err != nil {
		err = fmt.Errorf("failed to parse config file: %w", err)
		return
	}

	// Check for local config file and merge if exists
	// Extract the file extension and construct local config filename
	ext := filepath.Ext(configFile)
	if ext != "" {
		// Get the base name without extension
		baseName := strings.TrimSuffix(configFile, ext)
		// Construct local config filename: baseName + ".local" + ext
		localConfigFile := baseName + ".local" + ext
		if localConfig, localErr := os.ReadFile(localConfigFile); localErr == nil {
			// Parse local config and merge with base config
			var localSettings ShellTimeConfig
			if unmarshalErr := toml.Unmarshal(localConfig, &localSettings); unmarshalErr == nil {
				// Merge local settings into base config
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

	// Initialize CCOtel config with default port if enabled but port not set
	if config.CCOtel != nil && config.CCOtel.GRPCPort == 0 {
		config.CCOtel.GRPCPort = 54027 // default OTEL gRPC port
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
