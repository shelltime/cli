package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
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

// d2trackingConfig is a daemon config with code tracking on, which resync requires.
func d2trackingConfig(apiEndpoint string) model.ShellTimeConfig {
	enabled := true
	return model.ShellTimeConfig{Token: "tok", APIEndpoint: apiEndpoint, CodeTracking: &model.CodeTracking{Enabled: &enabled}}
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

	cfg := d2trackingConfig(server.URL)
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

	cfg := d2trackingConfig(server.URL)
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

	cfg := d2trackingConfig(server.URL)
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

// d2largeHeartbeatLine is one saved batch the size an editor sends after the daemon
// could not reach the server for a while: far over bufio.Scanner's 64 KiB line limit.
func d2largeHeartbeatLine(t *testing.T, prefix string, n int) string {
	t.Helper()
	payload := model.HeartbeatPayload{}
	for i := 0; i < n; i++ {
		payload.Heartbeats = append(payload.Heartbeats, model.HeartbeatData{
			HeartbeatID: fmt.Sprintf("%s-%d", prefix, i),
			Entity:      fmt.Sprintf("/home/dev/projects/shelltime/web/src/components/feature/file-%04d.tsx", i),
			Time:        int64(1_700_000_000 + i),
			Project:     "projects/web",
			Language:    "typescriptreact",
			Editor:      "vscode",
			Machine:     "dev-machine",
		})
	}
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	require.Greater(t, len(b), 64*1024)
	return string(b)
}

// A saved batch over 64 KiB stopped every resync, so nothing in the file was ever sent.
func TestResync_LineLargerThanScannerBuffer(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	logFile := d2writeHeartbeatLog(t, []string{
		d2largeHeartbeatLine(t, "big", 1000),
		`{"heartbeats":[{"heartbeatId":"small-1","entity":"/a.go","time":1,"project":"p"}]}`,
	})

	svc := NewHeartbeatResyncService(d2trackingConfig(server.URL))
	svc.resync(context.Background())

	assert.Equal(t, int32(3), hits.Load(), "both saved batches should be sent, the large one in two chunks")
	_, err := os.Stat(logFile)
	assert.True(t, os.IsNotExist(err), "log file should be removed after full success")
}

// Heartbeats the daemon saves while a resync is sending must survive the rewrite.
func TestResync_KeepsHeartbeatsSavedDuringResync(t *testing.T) {
	saved := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The first send triggers a save, as a failed live send would
		select {
		case <-saved:
		default:
			require.NoError(t, saveHeartbeatToFile(model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{
				{HeartbeatID: "live-1", Entity: "/live.go", Time: 3},
			}}))
			close(saved)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer server.Close()

	logFile := d2writeHeartbeatLog(t, []string{
		`{"heartbeats":[{"heartbeatId":"old-1","entity":"/a.go","time":1,"project":"p"}]}`,
	})

	svc := NewHeartbeatResyncService(d2trackingConfig(server.URL))
	svc.resync(context.Background())

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), `"old-1"`, "the batch that failed again is kept")
	assert.Contains(t, string(content), `"live-1"`, "the batch saved during the resync is kept")
}

// With code tracking off, saved heartbeats are kept for when it is turned on.
func TestResync_TrackingDisabledKeepsFile(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	line := `{"heartbeats":[{"heartbeatId":"off-1","entity":"/a.go","time":1}]}`
	logFile := d2writeHeartbeatLog(t, []string{line})

	svc := NewHeartbeatResyncService(model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL})
	svc.resync(context.Background())

	assert.Equal(t, int32(0), hits.Load())
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), `"off-1"`)
}

// Code tracking turned on after the daemon started, and a new token, apply to the next
// pass without a restart.
func TestResync_ReadsConfigFromDisk(t *testing.T) {
	var auth atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	logFile := d2writeHeartbeatLog(t, []string{`{"heartbeats":[{"heartbeatId":"later-1","entity":"/a.go","time":1}]}`})
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"),
		[]byte("token: new-token\napiEndpoint: "+server.URL+"\ncodeTracking:\n  enabled: true\n"), 0o600))

	// Started with tracking off and the old token
	svc := NewHeartbeatResyncService(model.ShellTimeConfig{Token: "old-token", APIEndpoint: server.URL})
	svc.SetConfigService(model.NewConfigService(configDir))
	svc.resync(context.Background())

	assert.Equal(t, "CLI new-token", auth.Load())
	_, err := os.Stat(logFile)
	assert.True(t, os.IsNotExist(err), "the batch should have been sent")
}

// Log cleanup can remove the file while a resync is sending; what was written after that
// is all new and must be kept.
func TestResync_KeepsHeartbeatsWhenFileWasReplaced(t *testing.T) {
	var logFile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, os.Remove(logFile))
		require.NoError(t, saveHeartbeatToFile(model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{
			{HeartbeatID: "after-cleanup", Entity: "/new.go", Time: 5},
		}}))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	logFile = d2writeHeartbeatLog(t, []string{`{"heartbeats":[{"heartbeatId":"old-1","entity":"/a.go","time":1}]}`})

	svc := NewHeartbeatResyncService(d2trackingConfig(server.URL))
	svc.resync(context.Background())

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), `"after-cleanup"`)
	assert.NotContains(t, string(content), `"old-1"`)
}

// A server before chunked inserts answers 200 with every row as an error when storing the
// batch failed; resync must keep the line, as the handler did.
func TestResync_KeepsBatchServerDidNotStore(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var payload model.HeartbeatPayload
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		_ = json.NewEncoder(w).Encode(model.HeartbeatResponse{Errors: len(payload.Heartbeats)})
	}))
	defer server.Close()

	line := `{"heartbeats":[{"heartbeatId":"unstored-1","entity":"/a.go","time":1}]}`
	logFile := d2writeHeartbeatLog(t, []string{line})

	svc := NewHeartbeatResyncService(d2trackingConfig(server.URL))
	svc.resync(context.Background())

	assert.Equal(t, int32(1), hits.Load())
	content, err := os.ReadFile(logFile)
	require.NoError(t, err, "the server stored nothing, so the line is kept")
	assert.Equal(t, line+"\n", string(content))
}

// Rows without an id or entity are rejected for good: a 200 rejecting only those is stored.
func TestResync_InvalidRowsAloneCountAsStored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(model.HeartbeatResponse{Success: true, Processed: 1, Errors: 1})
	}))
	defer server.Close()

	logFile := d2writeHeartbeatLog(t, []string{
		`{"heartbeats":[{"heartbeatId":"ok-1","entity":"/a.go","time":1},{"heartbeatId":"","entity":"/b.go","time":2}]}`,
	})

	svc := NewHeartbeatResyncService(d2trackingConfig(server.URL))
	svc.resync(context.Background())

	_, err := os.Stat(logFile)
	assert.True(t, os.IsNotExist(err), "log file should be removed")
}

// A large saved batch is resent in chunks, and only the chunks the server didn't store
// stay in the file, each as its own line.
func TestResync_SendsLargeLineInChunks(t *testing.T) {
	var mu sync.Mutex
	var sizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload model.HeartbeatPayload
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		mu.Lock()
		sizes = append(sizes, len(payload.Heartbeats))
		request := len(sizes)
		mu.Unlock()
		if request == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"insert failed"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(model.HeartbeatResponse{Success: true, Processed: len(payload.Heartbeats)})
	}))
	defer server.Close()

	logFile := d2writeHeartbeatLog(t, []string{d2largeHeartbeatLine(t, "big", 1200)})

	svc := NewHeartbeatResyncService(d2trackingConfig(server.URL))
	svc.resync(context.Background())

	assert.Equal(t, []int{500, 500, 200}, sizes)
	lines, err := readHeartbeatLogLines(logFile)
	require.NoError(t, err)
	require.Len(t, lines, 1, "only the failed chunk is kept")
	var kept model.HeartbeatPayload
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &kept))
	require.Len(t, kept.Heartbeats, 500)
	assert.Equal(t, "big-500", kept.Heartbeats[0].HeartbeatID)
	assert.Equal(t, "big-999", kept.Heartbeats[499].HeartbeatID)
}
