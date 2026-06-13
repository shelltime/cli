package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// d2writeHeartbeatLog points model.HEARTBEAT_LOG_FILE at a temp HOME and writes
// the given raw lines to the heartbeat log file the resync routine reads from.
// It returns the absolute log file path and registers env/var cleanup.
func d2writeHeartbeatLog(t *testing.T, lines []string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	logDir := filepath.Join(home, ".shelltime")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	logFile := filepath.Join(logDir, "coding-heartbeat.data.log")

	var content string
	for _, l := range lines {
		content += l + "\n"
	}
	require.NoError(t, os.WriteFile(logFile, []byte(content), 0o644))
	return logFile
}

// TestResync_SuccessRemovesFile drives resync through the happy path: a valid
// heartbeat line is sent to the (200) server, so successCount increments and
// the log file is removed (no failed lines remain).
func TestResync_SuccessRemovesFile(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/heartbeats", r.URL.Path)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	logFile := d2writeHeartbeatLog(t, []string{
		`{"heartbeats":[{"heartbeatId":"ok-1","entity":"/a.go","time":1,"project":"p"}]}`,
		`{"heartbeats":[{"heartbeatId":"ok-2","entity":"/b.go","time":2,"project":"p"}]}`,
	})

	cfg := model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	svc := NewHeartbeatResyncService(cfg)
	svc.resync(context.Background())

	assert.Equal(t, int32(2), hits.Load(), "both heartbeats should be sent")
	_, err := os.Stat(logFile)
	assert.True(t, os.IsNotExist(err), "log file should be removed after full success")
}

// TestResync_AllFailKeepsLines drives resync through the failure path: the
// server returns 500 for every send, so all lines are kept and the file is
// rewritten with the still-failing lines.
func TestResync_AllFailKeepsLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer server.Close()

	line1 := `{"heartbeats":[{"heartbeatId":"f-1","entity":"/a.go","time":1,"project":"p"}]}`
	line2 := `{"heartbeats":[{"heartbeatId":"f-2","entity":"/b.go","time":2,"project":"p"}]}`
	logFile := d2writeHeartbeatLog(t, []string{line1, line2})

	cfg := model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	svc := NewHeartbeatResyncService(cfg)
	svc.resync(context.Background())

	// File should still exist with both (failed) lines preserved.
	content, err := os.ReadFile(logFile)
	require.NoError(t, err, "log file should be kept when all sends fail")
	assert.Contains(t, string(content), `"f-1"`)
	assert.Contains(t, string(content), `"f-2"`)
}

// TestResync_MixedValidAndInvalidLines: invalid JSON lines are discarded; valid
// lines that fail to send are retained. Combined with a 500 server this keeps
// only the parseable line in the rewritten file.
func TestResync_MixedValidAndInvalidLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer server.Close()

	valid := `{"heartbeats":[{"heartbeatId":"v-1","entity":"/a.go","time":1,"project":"p"}]}`
	logFile := d2writeHeartbeatLog(t, []string{
		"this is not json",
		valid,
		`{also-bad}`,
	})

	cfg := model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	svc := NewHeartbeatResyncService(cfg)
	svc.resync(context.Background())

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	// Only the valid-but-failed line is retained; the malformed lines are gone.
	assert.Contains(t, string(content), `"v-1"`)
	assert.NotContains(t, string(content), "this is not json")
	assert.NotContains(t, string(content), "also-bad")
}

// TestRewriteLogFile_RemoveNonexistentIsNoError covers the empty-lines branch
// where the target file does not exist: os.Remove returns ErrNotExist which the
// function must treat as success.
func TestRewriteLogFile_RemoveNonexistentIsNoError(t *testing.T) {
	svc := NewHeartbeatResyncService(model.ShellTimeConfig{})
	missing := filepath.Join(t.TempDir(), "never-created.log")
	assert.NoError(t, svc.rewriteLogFile(missing, nil))
}
