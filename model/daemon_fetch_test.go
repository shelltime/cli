package model

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// daemonFetchFixture is a fake GitHub release server. Routes:
//
//	GET /repos/<owner>/<repo>/releases/latest          → {"tag_name": LatestTag}
//	GET /<owner>/<repo>/releases/download/<tag>/<file> → archive bytes (200) or 404
//	GET /<owner>/<repo>/releases/download/<tag>/checksums.txt → checksum line
type daemonFetchFixture struct {
	t           *testing.T
	LatestTag   string
	Archives    map[string][]byte // key: archiveName
	NotFoundTag string            // if a request comes in for this tag, respond 404 once
	hits404     atomic.Int32
	releaseHits atomic.Int32
	apiHits     atomic.Int32
}

func newDaemonFetchFixture(t *testing.T) *daemonFetchFixture {
	t.Helper()
	return &daemonFetchFixture{t: t, Archives: map[string][]byte{}}
}

func (f *daemonFetchFixture) start(t *testing.T) (apiURL, releaseURL string) {
	t.Helper()
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.apiHits.Add(1)
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			fmt.Fprintf(w, `{"tag_name": %q}`, f.LatestTag)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(apiSrv.Close)

	relSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.releaseHits.Add(1)
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		// expected: <owner>/<repo>/releases/download/<tag>/<file>
		if len(parts) < 6 {
			http.NotFound(w, r)
			return
		}
		tag := parts[4]
		file := parts[5]
		if f.NotFoundTag != "" && tag == f.NotFoundTag {
			f.hits404.Add(1)
			http.NotFound(w, r)
			return
		}
		if file == "checksums.txt" {
			var lines []string
			for name, body := range f.Archives {
				sum := sha256.Sum256(body)
				lines = append(lines, fmt.Sprintf("%s  %s", hex.EncodeToString(sum[:]), name))
			}
			fmt.Fprintln(w, strings.Join(lines, "\n"))
			return
		}
		body, ok := f.Archives[file]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(relSrv.Close)
	return apiSrv.URL, relSrv.URL
}

// pointURLsAt overrides the package-level GitHub base URLs for the duration
// of the test.
func pointURLsAt(t *testing.T, apiURL, releaseURL string) {
	t.Helper()
	prevAPI := githubAPIBaseURL
	prevRel := githubReleaseBaseURL
	githubAPIBaseURL = apiURL
	githubReleaseBaseURL = releaseURL
	t.Cleanup(func() {
		githubAPIBaseURL = prevAPI
		githubReleaseBaseURL = prevRel
	})
}

// makeArchiveWithDaemon returns archive bytes containing `shelltime-daemon`
// (and optionally `shelltime`) in the platform-appropriate format.
func makeArchiveWithDaemon(t *testing.T, includeDaemon bool) (name string, body []byte) {
	t.Helper()
	archiveName, err := BuildArchiveName(runtime.GOOS, runtime.GOARCH)
	require.NoError(t, err)

	if strings.HasSuffix(archiveName, ".zip") {
		return archiveName, makeZip(t, includeDaemon)
	}
	return archiveName, makeTarGz(t, includeDaemon)
}

func makeZip(t *testing.T, includeDaemon bool) []byte {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "out.zip")
	f, err := os.Create(tmp)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	cli, _ := zw.Create("shelltime")
	_, _ = cli.Write([]byte("CLI"))
	if includeDaemon {
		d, _ := zw.Create("shelltime-daemon")
		_, _ = d.Write([]byte("DAEMON_BODY"))
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	body, err := os.ReadFile(tmp)
	require.NoError(t, err)
	return body
}

func makeTarGz(t *testing.T, includeDaemon bool) []byte {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "out.tar.gz")
	f, err := os.Create(tmp)
	require.NoError(t, err)
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	write := func(name, body string) {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}
	write("shelltime", "CLI")
	if includeDaemon {
		write("shelltime-daemon", "DAEMON_BODY")
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	require.NoError(t, f.Close())
	body, err := os.ReadFile(tmp)
	require.NoError(t, err)
	return body
}

// isolateDaemonFetchEnv mirrors withIsolatedDaemonResolution but exposes the
// home dir for assertions and resets daemonFetchGOOS for the test.
func isolateDaemonFetchEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")

	prevHomebrew := daemonHomebrewSearchPaths
	daemonHomebrewSearchPaths = nil
	prevGOOS := daemonFetchGOOS
	daemonFetchGOOS = runtime.GOOS
	t.Cleanup(func() {
		daemonHomebrewSearchPaths = prevHomebrew
		daemonFetchGOOS = prevGOOS
	})
	return home
}

func TestEnsureDaemon_ExistingBinaryShortCircuits(t *testing.T) {
	home := isolateDaemonFetchEnv(t)
	existing := writeFakeDaemon(t, filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin"))

	fx := newDaemonFetchFixture(t)
	apiURL, relURL := fx.start(t)
	pointURLsAt(t, apiURL, relURL)

	got, err := EnsureDaemonBinary(context.Background(), "/usr/bin/shelltime", "v0.1.83")
	require.NoError(t, err)
	assert.Equal(t, existing, got)
	assert.Zero(t, fx.apiHits.Load(), "no API hit expected when binary already present")
	assert.Zero(t, fx.releaseHits.Load(), "no download hit expected when binary already present")
}

func TestEnsureDaemon_DownloadsWhenMissing(t *testing.T) {
	home := isolateDaemonFetchEnv(t)

	fx := newDaemonFetchFixture(t)
	fx.LatestTag = "v9.9.9"
	name, body := makeArchiveWithDaemon(t, true)
	fx.Archives[name] = body
	apiURL, relURL := fx.start(t)
	pointURLsAt(t, apiURL, relURL)

	cliPath := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime")
	got, err := EnsureDaemonBinary(context.Background(), cliPath, "v0.1.83")
	require.NoError(t, err)
	expected := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime-daemon")
	assert.Equal(t, expected, got)

	info, err := os.Stat(expected)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	contents, _ := os.ReadFile(expected)
	assert.Equal(t, "DAEMON_BODY", string(contents))
}

func TestEnsureDaemon_HomebrewAborts(t *testing.T) {
	isolateDaemonFetchEnv(t)

	fx := newDaemonFetchFixture(t)
	apiURL, relURL := fx.start(t)
	pointURLsAt(t, apiURL, relURL)

	_, err := EnsureDaemonBinary(context.Background(), "/opt/homebrew/bin/shelltime", "v0.1.83")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "brew reinstall")
	assert.Zero(t, fx.releaseHits.Load(), "no download attempt expected for Homebrew install")
}

func TestEnsureDaemon_WindowsAborts(t *testing.T) {
	isolateDaemonFetchEnv(t)
	daemonFetchGOOS = "windows"

	fx := newDaemonFetchFixture(t)
	apiURL, relURL := fx.start(t)
	pointURLsAt(t, apiURL, relURL)

	_, err := EnsureDaemonBinary(context.Background(), "C:\\Users\\me\\.shelltime\\bin\\shelltime.exe", "v0.1.83")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Windows")
	assert.Zero(t, fx.releaseHits.Load())
	assert.Zero(t, fx.apiHits.Load())
}

func TestEnsureDaemon_UsesCliVersionWhenSet(t *testing.T) {
	home := isolateDaemonFetchEnv(t)

	fx := newDaemonFetchFixture(t)
	name, body := makeArchiveWithDaemon(t, true)
	fx.Archives[name] = body

	var seenTag atomic.Value
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(apiSrv.Close)
	relSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 6 {
			seenTag.Store(parts[4])
		}
		file := parts[len(parts)-1]
		if file == "checksums.txt" {
			fmt.Fprintln(w)
			return
		}
		if b, ok := fx.Archives[file]; ok {
			_, _ = w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(relSrv.Close)
	pointURLsAt(t, apiSrv.URL, relSrv.URL)

	cliPath := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime")
	_, err := EnsureDaemonBinary(context.Background(), cliPath, "v0.94.5")
	require.NoError(t, err)
	assert.Equal(t, "v0.94.5", seenTag.Load())
}

func TestEnsureDaemon_ChecksumServer5xxAborts(t *testing.T) {
	home := isolateDaemonFetchEnv(t)

	name, body := makeArchiveWithDaemon(t, true)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(apiSrv.Close)
	relSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		file := parts[len(parts)-1]
		if file == "checksums.txt" {
			// Simulate a transient 5xx (or a tampered/blocked checksum
			// endpoint): the caller must NOT silently downgrade to an
			// unverified download.
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		if file == name {
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(relSrv.Close)
	pointURLsAt(t, apiSrv.URL, relSrv.URL)

	cliPath := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime")
	_, err := EnsureDaemonBinary(context.Background(), cliPath, "v0.1.83")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch checksum")
	// And the binary must not have been written.
	_, statErr := os.Stat(filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime-daemon"))
	assert.True(t, os.IsNotExist(statErr), "daemon binary should not be written on checksum failure")
}

func TestEnsureDaemon_PrefixesVForUnprefixedVersion(t *testing.T) {
	assert.Equal(t, "v0.94.5", normalizeDaemonTag("0.94.5"))
	assert.Equal(t, "v0.94.5", normalizeDaemonTag("v0.94.5"))
	assert.Equal(t, "", normalizeDaemonTag(""))
	assert.Equal(t, "", normalizeDaemonTag("dev"))
	assert.Equal(t, "", normalizeDaemonTag("  "))
}

func TestEnsureDaemon_FallsBackToLatestForDev(t *testing.T) {
	home := isolateDaemonFetchEnv(t)

	fx := newDaemonFetchFixture(t)
	fx.LatestTag = "v9.9.9"
	name, body := makeArchiveWithDaemon(t, true)
	fx.Archives[name] = body
	apiURL, relURL := fx.start(t)
	pointURLsAt(t, apiURL, relURL)

	cliPath := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime")
	_, err := EnsureDaemonBinary(context.Background(), cliPath, "dev")
	require.NoError(t, err)
	assert.Equal(t, int32(1), fx.apiHits.Load(), "expected one API hit to resolve latest tag")
}

func TestEnsureDaemon_NetworkErrorReturnsHelpful(t *testing.T) {
	home := isolateDaemonFetchEnv(t)

	// Servers immediately closed → connections fail.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	relSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	apiSrv.Close()
	relSrv.Close()
	pointURLsAt(t, apiSrv.URL, relSrv.URL)

	cliPath := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime")
	_, err := EnsureDaemonBinary(context.Background(), cliPath, "v0.1.83")
	require.Error(t, err)
}

func TestEnsureDaemon_ArchiveMissingDaemonBinary(t *testing.T) {
	home := isolateDaemonFetchEnv(t)

	fx := newDaemonFetchFixture(t)
	name, body := makeArchiveWithDaemon(t, false) // CLI only, no daemon
	fx.Archives[name] = body
	apiURL, relURL := fx.start(t)
	pointURLsAt(t, apiURL, relURL)

	cliPath := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime")
	_, err := EnsureDaemonBinary(context.Background(), cliPath, "v0.1.83")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not contain shelltime-daemon")
}

func TestEnsureDaemon_CreatesBinDir(t *testing.T) {
	home := isolateDaemonFetchEnv(t)
	// Deliberately don't create ~/.shelltime/bin upfront.
	_, err := os.Stat(filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin"))
	require.True(t, os.IsNotExist(err))

	fx := newDaemonFetchFixture(t)
	fx.LatestTag = "v9.9.9"
	name, body := makeArchiveWithDaemon(t, true)
	fx.Archives[name] = body
	apiURL, relURL := fx.start(t)
	pointURLsAt(t, apiURL, relURL)

	cliPath := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime")
	got, err := EnsureDaemonBinary(context.Background(), cliPath, "v0.1.83")
	require.NoError(t, err)
	_, err = os.Stat(got)
	require.NoError(t, err)
}

func TestEnsureDaemon_404FallsBackToLatest(t *testing.T) {
	home := isolateDaemonFetchEnv(t)

	fx := newDaemonFetchFixture(t)
	fx.LatestTag = "v9.9.9"
	fx.NotFoundTag = "v0.1.83" // pretend the tagged release was yanked
	name, body := makeArchiveWithDaemon(t, true)
	fx.Archives[name] = body
	apiURL, relURL := fx.start(t)
	pointURLsAt(t, apiURL, relURL)

	cliPath := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime")
	got, err := EnsureDaemonBinary(context.Background(), cliPath, "v0.1.83")
	require.NoError(t, err)
	expected := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin", "shelltime-daemon")
	assert.Equal(t, expected, got)
	assert.GreaterOrEqual(t, fx.hits404.Load(), int32(1), "expected at least one 404 before falling back")
}
