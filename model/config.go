package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var UserShellTimeConfig ShellTimeConfig

type ConfigService interface {
	ReadConfigFile(ctx context.Context) (ShellTimeConfig, error)
}

type configService struct {
	configFilePath string
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
}

func (cs *configService) ReadConfigFile(ctx context.Context) (config ShellTimeConfig, err error) {
	ctx, span := modelTracer.Start(ctx, "config.read")
	defer span.End()

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
	
	UserShellTimeConfig = config
	return
}
