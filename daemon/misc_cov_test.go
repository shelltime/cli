package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestX3RewriteLogFile_WritesRemainingLines covers the non-empty success path of
// HeartbeatResyncService.rewriteLogFile: lines are written to a temp file and
// atomically renamed over the target.
func TestX3RewriteLogFile_WritesRemainingLines(t *testing.T) {
	svc := NewHeartbeatResyncService(model.ShellTimeConfig{})
	logPath := filepath.Join(t.TempDir(), "heartbeat_failed.log")

	lines := []string{`{"id":"a"}`, `{"id":"b"}`}
	require.NoError(t, svc.rewriteLogFile(logPath, lines))

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	got := string(data)
	assert.Contains(t, got, `{"id":"a"}`)
	assert.Contains(t, got, `{"id":"b"}`)
	// Two lines, each newline-terminated.
	assert.Equal(t, 2, strings.Count(got, "\n"))

	// The temp file must not linger after the atomic rename.
	_, statErr := os.Stat(logPath + ".tmp")
	assert.True(t, os.IsNotExist(statErr), "temp file should be renamed away")
}

// TestX3WriteDebugFile_AppendsJSON covers the success path of
// AICodeOtelProcessor.writeDebugFile: the debug dir is created and the
// JSON-marshaled payload is appended with a timestamp header.
func TestX3WriteDebugFile_AppendsJSON(t *testing.T) {
	// Redirect TMPDIR so the debug file lands in an isolated, cleaned location.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	p.writeDebugFile("x3-debug.txt", map[string]any{"hello": "world", "n": 1})

	debugPath := filepath.Join(os.TempDir(), "shelltime", "x3-debug.txt")
	data, err := os.ReadFile(debugPath)
	require.NoError(t, err)
	got := string(data)
	assert.Contains(t, got, `"hello": "world"`)
	assert.Contains(t, got, "--- ", "should include the timestamp header")
}
