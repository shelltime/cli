package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadConfigFile_NoFileReturnsError(t *testing.T) {
	cs := NewConfigService(t.TempDir())
	_, err := cs.ReadConfigFile(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config file found")
}

func TestReadConfigFile_CacheAndSkipCache(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("token: first\napiEndpoint: https://api.example.com\n"), 0o644))

	cs := NewConfigService(dir)

	// First read populates the cache.
	c1, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "first", c1.Token)

	// Mutate the file on disk.
	require.NoError(t, os.WriteFile(cfgPath, []byte("token: second\napiEndpoint: https://api.example.com\n"), 0o644))

	// Cached read returns the original value (cache hit path).
	c2, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "first", c2.Token, "should serve cached config")

	// WithSkipCache re-reads from disk.
	c3, err := cs.ReadConfigFile(context.Background(), WithSkipCache())
	require.NoError(t, err)
	assert.Equal(t, "second", c3.Token, "WithSkipCache must bypass the cache")
}

func TestReadConfigFile_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	// Minimal config: defaults should fill in FlushCount/GCTime/WebEndpoint/etc.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("token: tok\n"), 0o644))

	cs := NewConfigService(dir)
	cfg, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 10, cfg.FlushCount, "default FlushCount")
	assert.Equal(t, 14, cfg.GCTime, "default GCTime")
	assert.Equal(t, "https://shelltime.xyz", cfg.WebEndpoint, "default WebEndpoint")
	assert.Equal(t, DefaultSocketPath, cfg.SocketPath, "default socket path")
	require.NotNil(t, cfg.DataMasking)
	assert.True(t, *cfg.DataMasking, "DataMasking defaults to true")
	require.NotNil(t, cfg.LogCleanup)
	require.NotNil(t, cfg.LogCleanup.Enabled)
	assert.True(t, *cfg.LogCleanup.Enabled)
	assert.EqualValues(t, 100, cfg.LogCleanup.ThresholdMB)
}

func TestReadConfigFile_AICodeOtelDefaultPort(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("token: tok\naiCodeOtel:\n  enabled: true\n"), 0o644))

	cs := NewConfigService(dir)
	cfg, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cfg.AICodeOtel)
	assert.Equal(t, 54027, cfg.AICodeOtel.GRPCPort, "default gRPC port applied when enabled but unset")
}

func TestReadConfigFile_DeprecatedCCOtelMigratesToAICodeOtel(t *testing.T) {
	dir := t.TempDir()
	// Only the deprecated ccotel field is set; it should migrate to AICodeOtel.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("token: tok\nccotel:\n  enabled: true\n  grpcPort: 9999\n"), 0o644))

	cs := NewConfigService(dir)
	cfg, err := cs.ReadConfigFile(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cfg.AICodeOtel, "ccotel should migrate to AICodeOtel")
	assert.Nil(t, cfg.CCOtel, "deprecated field cleared after migration")
	assert.Equal(t, 9999, cfg.AICodeOtel.GRPCPort)
}
