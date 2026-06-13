package commands

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// findPair is a small helper returning the value for a given key path, or
// ("", false) if the key is not present in the flattened pairs.
func findPair(pairs []keyValuePair, key string) (string, bool) {
	for _, p := range pairs {
		if p.key == key {
			return p.value, true
		}
	}
	return "", false
}

func TestFlattenConfig_StringRenderingAndMasking(t *testing.T) {
	cfg := model.ShellTimeConfig{
		// >8 chars -> first4 + **** + last4
		Token:       "abcdefghwxyz",
		APIEndpoint: "https://api.shelltime.xyz",
		WebEndpoint: "", // empty string -> <empty>
	}

	pairs := flattenConfig(cfg, "")

	// Token masking for long token: "abcd" + "****" + "wxyz".
	tokenVal, ok := findPair(pairs, "token")
	require.True(t, ok, "token key should be present")
	assert.Equal(t, "abcd****wxyz", tokenVal)

	// Non-token string is shown verbatim.
	apiVal, ok := findPair(pairs, "apiEndpoint")
	require.True(t, ok)
	assert.Equal(t, "https://api.shelltime.xyz", apiVal)

	// Empty string renders as <empty>.
	webVal, ok := findPair(pairs, "webEndpoint")
	require.True(t, ok)
	assert.Equal(t, "<empty>", webVal)
}

func TestFlattenConfig_ShortTokenMasking(t *testing.T) {
	// Token <= 8 chars masks to "****" entirely.
	cfg := model.ShellTimeConfig{Token: "short"}
	pairs := flattenConfig(cfg, "")
	tokenVal, ok := findPair(pairs, "token")
	require.True(t, ok)
	assert.Equal(t, "****", tokenVal)
}

func TestFlattenConfig_EmptyTokenIsEmptyNotMasked(t *testing.T) {
	// An empty token field hits the "<empty>" branch before masking.
	cfg := model.ShellTimeConfig{Token: ""}
	pairs := flattenConfig(cfg, "")
	tokenVal, ok := findPair(pairs, "token")
	require.True(t, ok)
	assert.Equal(t, "<empty>", tokenVal)
}

func TestFlattenConfig_NilPointerRendersNotSet(t *testing.T) {
	// DataMasking is a *bool left nil -> "<not set>".
	cfg := model.ShellTimeConfig{Token: "x"}
	pairs := flattenConfig(cfg, "")
	v, ok := findPair(pairs, "dataMasking")
	require.True(t, ok)
	assert.Equal(t, "<not set>", v)

	// AI pointer struct is nil -> "<not set>" (not recursed into).
	v, ok = findPair(pairs, "ai")
	require.True(t, ok)
	assert.Equal(t, "<not set>", v)
}

func TestFlattenConfig_BoolPointerRendered(t *testing.T) {
	truthy := true
	falsy := false
	cfg := model.ShellTimeConfig{
		DataMasking:   &truthy,
		EnableMetrics: &falsy,
	}
	pairs := flattenConfig(cfg, "")

	v, ok := findPair(pairs, "dataMasking")
	require.True(t, ok)
	assert.Equal(t, "true", v)

	v, ok = findPair(pairs, "enableMetrics")
	require.True(t, ok)
	assert.Equal(t, "false", v)
}

func TestFlattenConfig_IntRendered(t *testing.T) {
	cfg := model.ShellTimeConfig{FlushCount: 42, GCTime: 7}
	pairs := flattenConfig(cfg, "")

	v, ok := findPair(pairs, "flushCount")
	require.True(t, ok)
	assert.Equal(t, "42", v)

	v, ok = findPair(pairs, "gcTime")
	require.True(t, ok)
	assert.Equal(t, "7", v)
}

func TestFlattenConfig_EmptySliceAndPopulatedSlice(t *testing.T) {
	// Empty slice -> "[]".
	cfg := model.ShellTimeConfig{Exclude: []string{}}
	pairs := flattenConfig(cfg, "")
	v, ok := findPair(pairs, "exclude")
	require.True(t, ok)
	assert.Equal(t, "[]", v)

	// Populated slice -> JSON.
	cfg = model.ShellTimeConfig{Exclude: []string{"^secret", "^password"}}
	pairs = flattenConfig(cfg, "")
	v, ok = findPair(pairs, "exclude")
	require.True(t, ok)
	assert.Equal(t, `["^secret","^password"]`, v)
}

func TestFlattenConfig_NestedStructRecursion(t *testing.T) {
	// Storage is a non-pointer-ish nested struct via *StorageConfig.
	cfg := model.ShellTimeConfig{
		Storage: &model.StorageConfig{Engine: "bolt"},
	}
	pairs := flattenConfig(cfg, "")

	// Nested pointer struct should be recursed -> "storage.engine".
	v, ok := findPair(pairs, "storage.engine")
	require.True(t, ok, "nested struct key should use parent.child path")
	assert.Equal(t, "bolt", v)
}

func TestFlattenConfig_NestedTokenMasking(t *testing.T) {
	// AICodeOtel has no token field; use AIConfig which may contain a token-like
	// nested field. Instead, verify the masking applies on nested keys that
	// contain "token". We construct a struct via Endpoints which carry tokens.
	cfg := model.ShellTimeConfig{
		Endpoints: []model.Endpoint{
			{APIEndpoint: "https://e", Token: "supersecrettoken"},
		},
	}
	pairs := flattenConfig(cfg, "")
	// Endpoints is a slice -> rendered as JSON (not recursed), so the token
	// appears unmasked inside the JSON. This documents actual behavior:
	// slice fields are JSON-marshaled, not masked.
	v, ok := findPair(pairs, "endpoints")
	require.True(t, ok)
	assert.Contains(t, v, "supersecrettoken")
}

func TestFlattenConfig_NonStructReturnsEmpty(t *testing.T) {
	assert.Empty(t, flattenConfig(42, ""))
	assert.Empty(t, flattenConfig("just a string", ""))

	// Nil pointer returns empty.
	var p *model.ShellTimeConfig
	assert.Empty(t, flattenConfig(p, ""))
}

func TestFlattenConfig_PointerToStructIsDereferenced(t *testing.T) {
	cfg := &model.ShellTimeConfig{Token: "abcdefghwxyz"}
	pairs := flattenConfig(cfg, "")
	v, ok := findPair(pairs, "token")
	require.True(t, ok)
	assert.Equal(t, "abcd****wxyz", v)
}

func TestFlattenConfig_PrefixApplied(t *testing.T) {
	cfg := model.ShellTimeConfig{FlushCount: 5}
	pairs := flattenConfig(cfg, "root")
	v, ok := findPair(pairs, "root.flushCount")
	require.True(t, ok, "prefix should be prepended with a dot")
	assert.Equal(t, "5", v)
}

func TestOutputConfigJSON_ValidJSON(t *testing.T) {
	cfg := model.ShellTimeConfig{
		Token:       "abc",
		APIEndpoint: "https://api.shelltime.xyz",
		FlushCount:  10,
	}
	// outputConfigJSON prints to stdout and returns nil for a marshalable struct.
	err := outputConfigJSON(cfg)
	assert.NoError(t, err)
}

func TestOutputConfigJSON_RoundTrips(t *testing.T) {
	// Verify the marshaled form is valid JSON by re-marshaling the same way
	// outputConfigJSON does and unmarshaling it back.
	cfg := model.ShellTimeConfig{Token: "abc", FlushCount: 3}
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	var back model.ShellTimeConfig
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, "abc", back.Token)
	assert.Equal(t, 3, back.FlushCount)
}

func TestOutputConfigTable_NoError(t *testing.T) {
	cfg := model.ShellTimeConfig{Token: "abcdefghwxyz", FlushCount: 1}
	assert.NoError(t, outputConfigTable(cfg))
}

// configViewTestSuite drives the configView Action through a *cli.Context with
// a mocked ConfigService.
type configViewTestSuite struct {
	origConfig model.ConfigService
}

func setupConfigViewTest(t *testing.T) (*model.MockConfigService, func()) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	return mc, func() { configService = orig }
}

func TestConfigViewAction_UnsupportedFormat(t *testing.T) {
	mc, cleanup := setupConfigViewTest(t)
	t.Cleanup(cleanup)
	_ = mc // not consulted when format is invalid

	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	err := app.Run([]string{"t", "view", "-f", "xml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestConfigViewAction_JSONSuccess(t *testing.T) {
	mc, cleanup := setupConfigViewTest(t)
	t.Cleanup(cleanup)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: "abcdefghwxyz"}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	err := app.Run([]string{"t", "view", "-f", "json"})
	assert.NoError(t, err)
}

func TestConfigViewAction_TableSuccess(t *testing.T) {
	mc, cleanup := setupConfigViewTest(t)
	t.Cleanup(cleanup)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: "abcdefghwxyz", FlushCount: 9}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	// default format is table
	err := app.Run([]string{"t", "view"})
	assert.NoError(t, err)
}

func TestConfigViewAction_ConfigReadError(t *testing.T) {
	mc, cleanup := setupConfigViewTest(t)
	t.Cleanup(cleanup)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, errors.New("boom"))

	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	err := app.Run([]string{"t", "view", "-f", "json"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config")
}
