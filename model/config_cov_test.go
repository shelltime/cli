package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeConfig_AllOverrides exercises every branch of mergeConfig by supplying
// a local config that overrides each field of a fully-populated base config.
func TestMergeConfig_AllOverrides(t *testing.T) {
	truthy := true
	base := &ShellTimeConfig{
		Token:       "base-tok",
		APIEndpoint: "https://base",
		WebEndpoint: "https://baseweb",
		FlushCount:  1,
		GCTime:      1,
		SocketPath:  "/tmp/base.sock",
	}
	on := true
	local := &ShellTimeConfig{
		Token:         "local-tok",
		APIEndpoint:   "https://local",
		WebEndpoint:   "https://localweb",
		FlushCount:    20,
		GCTime:        30,
		DataMasking:   &truthy,
		EnableMetrics: &truthy,
		Encrypted:     &truthy,
		AI:            &AIConfig{},
		Endpoints:     []Endpoint{{Token: "e", APIEndpoint: "https://ep"}},
		Exclude:       []string{"secret"},
		CCUsage:       &CCUsage{Enabled: &on},
		AICodeOtel:    &AICodeOtel{Enabled: &on},
		LogCleanup:    &LogCleanup{Enabled: &truthy, ThresholdMB: 42},
		SocketPath:    "/tmp/local.sock",
		CodeTracking:  &CodeTracking{Token: "ct"},
	}

	mergeConfig(base, local)

	assert.Equal(t, "local-tok", base.Token)
	assert.Equal(t, "https://local", base.APIEndpoint)
	assert.Equal(t, "https://localweb", base.WebEndpoint)
	assert.Equal(t, 20, base.FlushCount)
	assert.Equal(t, 30, base.GCTime)
	require.NotNil(t, base.DataMasking)
	require.NotNil(t, base.EnableMetrics)
	require.NotNil(t, base.Encrypted)
	require.NotNil(t, base.AI)
	require.Len(t, base.Endpoints, 1)
	require.Len(t, base.Exclude, 1)
	require.NotNil(t, base.CCUsage)
	require.NotNil(t, base.AICodeOtel)
	require.NotNil(t, base.LogCleanup)
	assert.EqualValues(t, 42, base.LogCleanup.ThresholdMB)
	assert.Equal(t, "/tmp/local.sock", base.SocketPath)
	require.NotNil(t, base.CodeTracking)
}

// TestMergeConfig_CCOtelMigration covers the deprecated CCOtel -> AICodeOtel
// migration branch inside mergeConfig (local has CCOtel, no AICodeOtel).
func TestMergeConfig_CCOtelMigration(t *testing.T) {
	base := &ShellTimeConfig{}
	on := true
	local := &ShellTimeConfig{
		CCOtel: &AICodeOtel{Enabled: &on, GRPCPort: 1234},
	}
	mergeConfig(base, local)
	require.NotNil(t, base.AICodeOtel, "CCOtel should migrate into AICodeOtel on base")
	assert.Equal(t, 1234, base.AICodeOtel.GRPCPort)
}

// TestMergeConfig_NoOverrides ensures zero-valued local fields leave base intact.
func TestMergeConfig_NoOverrides(t *testing.T) {
	base := &ShellTimeConfig{Token: "keep", FlushCount: 7, GCTime: 9, SocketPath: "/keep"}
	mergeConfig(base, &ShellTimeConfig{})
	assert.Equal(t, "keep", base.Token)
	assert.Equal(t, 7, base.FlushCount)
	assert.Equal(t, 9, base.GCTime)
	assert.Equal(t, "/keep", base.SocketPath)
}

// TestReadConfigFile_MergesLocalOverBase writes both a base YAML config and a
// local override, then asserts the merged result reflects the local values plus
// applied defaults (covers the local-file read + merge branch of ReadConfigFile).
func TestReadConfigFile_MergesLocalOverBase(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("token: base-tok\napiEndpoint: https://base\nflushCount: 50\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.yaml"),
		[]byte("token: local-tok\nexclude:\n  - secret-cmd\n"), 0o644))

	cs := NewConfigService(dir)
	cfg, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "local-tok", cfg.Token, "local token overrides base")
	assert.Equal(t, "https://base", cfg.APIEndpoint, "base value preserved when local omits it")
	assert.Equal(t, 50, cfg.FlushCount, "base flushCount kept")
	require.Contains(t, cfg.Exclude, "secret-cmd")
}

// TestReadConfigFile_InvalidLocalIsIgnored covers the branch where the local
// config fails to parse: the warning is logged and base config is still used.
func TestReadConfigFile_InvalidLocalIsIgnored(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("token: base-tok\napiEndpoint: https://base\n"), 0o644))
	// Invalid YAML in local override.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.yaml"),
		[]byte(":::not yaml:::\n  - ["), 0o644))

	cs := NewConfigService(dir)
	cfg, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err, "invalid local config must not fail the read")
	assert.Equal(t, "base-tok", cfg.Token)
}

// TestReadConfigFile_FlushCountFloor covers the "FlushCount < 3 -> 3" branch.
func TestReadConfigFile_FlushCountFloor(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("token: tok\nflushCount: 1\n"), 0o644))

	cs := NewConfigService(dir)
	cfg, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.FlushCount, "flushCount below 3 is raised to the floor of 3")
}

// TestReadConfigFile_TOMLBaseWithLogCleanupPartial covers a TOML base file with a
// LogCleanup table missing the threshold, hitting the "fill defaults into an
// existing LogCleanup" branch.
func TestReadConfigFile_TOMLBaseWithLogCleanupPartial(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("token = \"tok\"\n[logCleanup]\n"), 0o644))

	cs := NewConfigService(dir)
	cfg, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cfg.LogCleanup)
	require.NotNil(t, cfg.LogCleanup.Enabled)
	assert.True(t, *cfg.LogCleanup.Enabled, "Enabled defaults to true when omitted")
	assert.EqualValues(t, 100, cfg.LogCleanup.ThresholdMB, "ThresholdMB defaults to 100 when zero")
}
