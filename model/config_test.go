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
	baseConfig := `token = 'base-token'
apiEndpoint = 'https://api.base.com'
webEndpoint = 'https://base.com'
flushCount = 5
gcTime = 7
dataMasking = false
enableMetrics = false
encrypted = false`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	// Create local config file that overrides some settings
	localConfigPath := filepath.Join(tmpDir, "config.local.toml")
	localConfig := `token = 'local-token'
apiEndpoint = 'https://api.local.com'
flushCount = 10
dataMasking = true`
	err = os.WriteFile(localConfigPath, []byte(localConfig), 0644)
	require.NoError(t, err)

	// Test reading config with local override (now uses directory path)
	cs := NewConfigService(tmpDir)
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
	baseConfig := `token = 'base-token'
apiEndpoint = 'https://api.base.com'
webEndpoint = 'https://base.com'
flushCount = 5
gcTime = 7`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	// Test reading config without local file (now uses directory path)
	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify base config values are used
	assert.Equal(t, "base-token", config.Token)
	assert.Equal(t, "https://api.base.com", config.APIEndpoint)
	assert.Equal(t, "https://base.com", config.WebEndpoint)
	assert.Equal(t, 5, config.FlushCount)
	assert.Equal(t, 7, config.GCTime)
}

func TestReadYAMLConfig(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create YAML config file
	yamlConfigPath := filepath.Join(tmpDir, "config.yaml")
	yamlConfig := `token: yaml-token
apiEndpoint: https://api.yaml.com
webEndpoint: https://yaml.com
flushCount: 15
gcTime: 21`
	err = os.WriteFile(yamlConfigPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "yaml-token", config.Token)
	assert.Equal(t, "https://api.yaml.com", config.APIEndpoint)
	assert.Equal(t, "https://yaml.com", config.WebEndpoint)
	assert.Equal(t, 15, config.FlushCount)
	assert.Equal(t, 21, config.GCTime)
}

func TestReadYMLConfig(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create YML config file
	ymlConfigPath := filepath.Join(tmpDir, "config.yml")
	ymlConfig := `token: yml-token
apiEndpoint: https://api.yml.com
flushCount: 12`
	err = os.WriteFile(ymlConfigPath, []byte(ymlConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "yml-token", config.Token)
	assert.Equal(t, "https://api.yml.com", config.APIEndpoint)
	assert.Equal(t, 12, config.FlushCount)
}

func TestYAMLConfigWithLocalOverride(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create base YAML config file
	yamlConfigPath := filepath.Join(tmpDir, "config.yaml")
	yamlConfig := `token: base-yaml-token
apiEndpoint: https://api.base.com
flushCount: 5`
	err = os.WriteFile(yamlConfigPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	// Create local YAML config file
	localYamlConfigPath := filepath.Join(tmpDir, "config.local.yaml")
	localYamlConfig := `token: local-yaml-token
flushCount: 20`
	err = os.WriteFile(localYamlConfigPath, []byte(localYamlConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify local config overrides base config
	assert.Equal(t, "local-yaml-token", config.Token)
	assert.Equal(t, 20, config.FlushCount)
	// Base value should be kept
	assert.Equal(t, "https://api.base.com", config.APIEndpoint)
}

func TestConfigPriority_YAMLOverTOML(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create both TOML and YAML config files
	tomlConfigPath := filepath.Join(tmpDir, "config.toml")
	tomlConfig := `token = 'toml-token'
apiEndpoint = 'https://api.toml.com'`
	err = os.WriteFile(tomlConfigPath, []byte(tomlConfig), 0644)
	require.NoError(t, err)

	yamlConfigPath := filepath.Join(tmpDir, "config.yaml")
	yamlConfig := `token: yaml-token
apiEndpoint: https://api.yaml.com`
	err = os.WriteFile(yamlConfigPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// YAML should take priority over TOML
	assert.Equal(t, "yaml-token", config.Token)
	assert.Equal(t, "https://api.yaml.com", config.APIEndpoint)
}

func TestConfigPriority_YAMLOverYML(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create both .yaml and .yml config files
	ymlConfigPath := filepath.Join(tmpDir, "config.yml")
	ymlConfig := `token: yml-token`
	err = os.WriteFile(ymlConfigPath, []byte(ymlConfig), 0644)
	require.NoError(t, err)

	yamlConfigPath := filepath.Join(tmpDir, "config.yaml")
	yamlConfig := `token: yaml-token`
	err = os.WriteFile(yamlConfigPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// .yaml should take priority over .yml
	assert.Equal(t, "yaml-token", config.Token)
}

func TestConfigPriority_LocalYAMLOverLocalTOML(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create base config
	yamlConfigPath := filepath.Join(tmpDir, "config.yaml")
	yamlConfig := `token: base-token
flushCount: 5`
	err = os.WriteFile(yamlConfigPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	// Create both local TOML and local YAML
	localTomlPath := filepath.Join(tmpDir, "config.local.toml")
	localToml := `token = 'local-toml-token'
flushCount = 10`
	err = os.WriteFile(localTomlPath, []byte(localToml), 0644)
	require.NoError(t, err)

	localYamlPath := filepath.Join(tmpDir, "config.local.yaml")
	localYaml := `token: local-yaml-token
flushCount: 20`
	err = os.WriteFile(localYamlPath, []byte(localYaml), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// local.yaml should take priority over local.toml
	assert.Equal(t, "local-yaml-token", config.Token)
	assert.Equal(t, 20, config.FlushCount)
}

func TestCrossFormatConfig_YAMLBaseWithTOMLLocal(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create YAML base config
	yamlConfigPath := filepath.Join(tmpDir, "config.yaml")
	yamlConfig := `token: yaml-base-token
apiEndpoint: https://api.yaml.com
flushCount: 5`
	err = os.WriteFile(yamlConfigPath, []byte(yamlConfig), 0644)
	require.NoError(t, err)

	// Create TOML local config (no YAML local exists)
	localTomlPath := filepath.Join(tmpDir, "config.local.toml")
	localToml := `token = 'toml-local-token'
flushCount = 15`
	err = os.WriteFile(localTomlPath, []byte(localToml), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Local TOML should override YAML base
	assert.Equal(t, "toml-local-token", config.Token)
	assert.Equal(t, 15, config.FlushCount)
	// Base value should be kept
	assert.Equal(t, "https://api.yaml.com", config.APIEndpoint)
}

func TestLogCleanupDefaults(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create config file without LogCleanup section
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `token = 'test-token'
apiEndpoint = 'https://api.test.com'`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
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
	baseConfig := `token = 'test-token'
apiEndpoint = 'https://api.test.com'

[logCleanup]
enabled = false
thresholdMB = 200`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
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
			config: `token = 'test-token'
[logCleanup]
enabled = false`,
			expectedEnabled:   false,
			expectedThreshold: 100, // default
		},
		{
			name: "Only threshold set",
			config: `token = 'test-token'
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

			cs := NewConfigService(tmpDir)
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
	baseConfig := `token = 'base-token'
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

	cs := NewConfigService(tmpDir)
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
	baseConfig := `token = 'base-token'
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

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify local config overrides base CodeTracking
	require.NotNil(t, config.CodeTracking, "CodeTracking should be present")
	assert.True(t, *config.CodeTracking.Enabled, "CodeTracking.Enabled should be overridden by local config")
}

func TestCodeTrackingCustomEndpointAndToken(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create config file with custom CodeTracking endpoint and token
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `token = 'global-token'
apiEndpoint = 'https://api.global.com'

[codeTracking]
enabled = true
apiEndpoint = 'https://api.custom-heartbeat.com'
token = 'custom-heartbeat-token'`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify global config values
	assert.Equal(t, "global-token", config.Token)
	assert.Equal(t, "https://api.global.com", config.APIEndpoint)

	// Verify CodeTracking custom values
	require.NotNil(t, config.CodeTracking, "CodeTracking should be present")
	assert.True(t, *config.CodeTracking.Enabled)
	assert.Equal(t, "https://api.custom-heartbeat.com", config.CodeTracking.APIEndpoint)
	assert.Equal(t, "custom-heartbeat-token", config.CodeTracking.Token)
}

func TestCodeTrackingPartialCustomConfig(t *testing.T) {
	testCases := []struct {
		name                string
		config              string
		expectedAPIEndpoint string
		expectedToken       string
	}{
		{
			name: "Only custom apiEndpoint",
			config: `token = 'global-token'
apiEndpoint = 'https://api.global.com'

[codeTracking]
enabled = true
apiEndpoint = 'https://api.custom.com'`,
			expectedAPIEndpoint: "https://api.custom.com",
			expectedToken:       "", // empty, should fall back to global
		},
		{
			name: "Only custom token",
			config: `token = 'global-token'
apiEndpoint = 'https://api.global.com'

[codeTracking]
enabled = true
token = 'custom-token'`,
			expectedAPIEndpoint: "", // empty, should fall back to global
			expectedToken:       "custom-token",
		},
		{
			name: "No custom endpoint or token",
			config: `token = 'global-token'
apiEndpoint = 'https://api.global.com'

[codeTracking]
enabled = true`,
			expectedAPIEndpoint: "",
			expectedToken:       "",
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

			cs := NewConfigService(tmpDir)
			config, err := cs.ReadConfigFile(context.Background())
			require.NoError(t, err)

			require.NotNil(t, config.CodeTracking, "CodeTracking should be present")
			assert.Equal(t, tc.expectedAPIEndpoint, config.CodeTracking.APIEndpoint)
			assert.Equal(t, tc.expectedToken, config.CodeTracking.Token)
		})
	}
}

func TestCodeTrackingMergeEndpointFromLocal(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir, err := os.MkdirTemp("", "shelltime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create base config file with CodeTracking
	baseConfigPath := filepath.Join(tmpDir, "config.toml")
	baseConfig := `token = 'base-token'
apiEndpoint = 'https://api.base.com'

[codeTracking]
enabled = true
apiEndpoint = 'https://api.base-heartbeat.com'
token = 'base-heartbeat-token'`
	err = os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	require.NoError(t, err)

	// Create local config file that overrides CodeTracking endpoint and token
	localConfigPath := filepath.Join(tmpDir, "config.local.toml")
	localConfig := `[codeTracking]
enabled = true
apiEndpoint = 'https://api.local-heartbeat.com'
token = 'local-heartbeat-token'`
	err = os.WriteFile(localConfigPath, []byte(localConfig), 0644)
	require.NoError(t, err)

	cs := NewConfigService(tmpDir)
	config, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	// Verify local config overrides base CodeTracking
	require.NotNil(t, config.CodeTracking, "CodeTracking should be present")
	assert.True(t, *config.CodeTracking.Enabled)
	assert.Equal(t, "https://api.local-heartbeat.com", config.CodeTracking.APIEndpoint)
	assert.Equal(t, "local-heartbeat-token", config.CodeTracking.Token)
}
