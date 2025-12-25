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

func TestLogCleanupDefaults(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create config file without LogCleanup section
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `Token = 'test-token'
APIEndpoint = 'https://api.test.com'`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(baseConfigPath)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify LogCleanup defaults are applied
	require.NotNil(t, config.LogCleanup, "LogCleanup should be initialized with defaults")
	assert.True(t, *config.LogCleanup.Enabled, "LogCleanup.Enabled should default to true")
	assert.Equal(t, int64(100), config.LogCleanup.ThresholdMB, "LogCleanup.ThresholdMB should default to 100")
}

func TestLogCleanupCustomValues(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create config file with custom LogCleanup settings
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `Token = 'test-token'
APIEndpoint = 'https://api.test.com'

[logCleanup]
enabled = false
thresholdMB = 200`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(baseConfigPath)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify custom LogCleanup values are used
	require.NotNil(t, config.LogCleanup, "LogCleanup should be present")
	assert.False(t, *config.LogCleanup.Enabled, "LogCleanup.Enabled should be false")
	assert.Equal(t, int64(200), config.LogCleanup.ThresholdMB, "LogCleanup.ThresholdMB should be 200")
}

func TestLogCleanupPartialConfig(t *testing.T) {
	testCases := []struct {
		name              string
		config            string
		expectedEnabled   bool
		expectedThreshold int64
	}{
		{
			name: "Only enabled set to false",
			config: `Token = 'test-token'
[logCleanup]
enabled = false`,
			expectedEnabled:   false,
			expectedThreshold: 100, // default
		},
		{
			name: "Only threshold set",
			config: `Token = 'test-token'
[logCleanup]
thresholdMB = 50`,
			expectedEnabled:   true, // default
			expectedThreshold: 50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			baseConfigPath := filepath.Join(tmpDir, "config.toml")
			err = os.WriteFile(baseConfigPath, []byte(tc.config), 0644)
			require.NoError(t, err)

			cs := NewConfigService(baseConfigPath)
			config, err := cs.ReadConfigFile(context.Background())
			require.NoError(t, err)

			require.NotNil(t, config.LogCleanup, "LogCleanup should be present")
			assert.Equal(t, tc.expectedEnabled, *config.LogCleanup.Enabled, "LogCleanup.Enabled mismatch")
			assert.Equal(t, tc.expectedThreshold, config.LogCleanup.ThresholdMB, "LogCleanup.ThresholdMB mismatch")
		})
	}
}

func TestLogCleanupMergeFromLocal(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create base config file with LogCleanup
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `Token = 'base-token'
[logCleanup]
enabled = true
thresholdMB = 100`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	// Create local config file that overrides LogCleanup
	localConfigPath := filepath.Join(tmpDir, "config.local.toml")
	localConfig := `[logCleanup]
enabled = false
thresholdMB = 500`
	err = os.WriteFile(localConfigPath, []byte(localConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(baseConfigPath)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify local config overrides base LogCleanup
	require.NotNil(t, config.LogCleanup, "LogCleanup should be present")
	assert.False(t, *config.LogCleanup.Enabled, "LogCleanup.Enabled should be overridden by local config")
	assert.Equal(t, int64(500), config.LogCleanup.ThresholdMB, "LogCleanup.ThresholdMB should be overridden by local config")
}

func TestCodeTrackingMergeFromLocal(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create base config file with CodeTracking disabled
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `Token = 'base-token'
[codeTracking]
enabled = false`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	// Create local config file that enables CodeTracking
	localConfigPath := filepath.Join(tmpDir, "config.local.toml")
	localConfig := `[codeTracking]
enabled = true`
	err = os.WriteFile(localConfigPath, []byte(localConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(baseConfigPath)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify local config overrides base CodeTracking
	require.NotNil(t, config.CodeTracking, "CodeTracking should be present")
	assert.True(t, *config.CodeTracking.Enabled, "CodeTracking.Enabled should be overridden by local config")
}
