package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadConfigFileWithLocal(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create base config file
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `Token = 'base-token'
APIEndpoint = 'https://api.base.com'
WebEndpoint = 'https://base.com'
FlushCount = 5
GCTime = 7
dataMasking = false
enableMetrics = false
encrypted = false`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	// Create local config file that overrides some settings
	localConfigPath := filepath.Join(tmpDir, "config.local.toml")
	localConfig := `Token = 'local-token'
APIEndpoint = 'https://api.local.com'
FlushCount = 10
dataMasking = true`
	err = os.WriteFile(localConfigPath, []byte(localConfig), 0644)
	require.NoError(t, err)

	// Test reading config with local override
	cs := NewConfigService(baseConfigPath)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify local config overrides base config
	assert.Equal(t, "local-token", config.Token, "Token should be overridden by local config")
	assert.Equal(t, "https://api.local.com", config.APIEndpoint, "APIEndpoint should be overridden by local config")
	assert.Equal(t, 10, config.FlushCount, "FlushCount should be overridden by local config")
	assert.True(t, *config.DataMasking, "DataMasking should be overridden by local config")

	// Verify base config values that weren't overridden
	assert.Equal(t, "https://base.com", config.WebEndpoint, "WebEndpoint should keep base value")
	assert.Equal(t, 7, config.GCTime, "GCTime should keep base value")
	assert.False(t, *config.EnableMetrics, "EnableMetrics should keep base value")
	assert.False(t, *config.Encrypted, "Encrypted should keep base value")
}

func TestReadConfigFileWithoutLocal(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create only base config file (no local file)
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `Token = 'base-token'
APIEndpoint = 'https://api.base.com'
WebEndpoint = 'https://base.com'
FlushCount = 5
GCTime = 7`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	// Test reading config without local file
	cs := NewConfigService(baseConfigPath)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify base config values are used
	assert.Equal(t, "base-token", config.Token)
	assert.Equal(t, "https://api.base.com", config.APIEndpoint)
	assert.Equal(t, "https://base.com", config.WebEndpoint)
	assert.Equal(t, 5, config.FlushCount)
	assert.Equal(t, 7, config.GCTime)
}

func TestReadConfigFileWithDifferentExtensions(t *testing.T) {
	testCases := []struct {
		name       string
		configFile string
		localFile  string
	}{
		{
			name:       "TOML files",
			configFile: "config.toml",
			localFile:  "config.local.toml",
		},
		{
			name:       "Custom config name",
			configFile: "shelltime-config.toml",
			localFile:  "shelltime-config.local.toml",
		},
		{
			name:       "Different extension",
			configFile: "settings.conf",
			localFile:  "settings.local.conf",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory for test configs
			tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Create base config file
			baseConfigPath := filepath.Join(tmpDir, tc.configFile)
			baseConfig := `Token = 'base-token'
APIEndpoint = 'https://api.base.com'
FlushCount = 5`
			err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
			require.NoError(t, err)

			// Create local config file
			localConfigPath := filepath.Join(tmpDir, tc.localFile)
			localConfig := `Token = 'local-token'
FlushCount = 10`
			err = os.WriteFile(localConfigPath, []byte(localConfig), 0644)
			require.NoError(t, err)

			// Test reading config with local override
			cs := NewConfigService(baseConfigPath)
			config, err := cs.ReadConfigFile(context.Background())
			require.NoError(t, err)

			// Verify local config overrides base config
			assert.Equal(t, "local-token", config.Token, "Token should be overridden by local config for %s", tc.name)
			assert.Equal(t, 10, config.FlushCount, "FlushCount should be overridden by local config for %s", tc.name)
			assert.Equal(t, "https://api.base.com", config.APIEndpoint, "APIEndpoint should keep base value for %s", tc.name)
		})
	}
}
