package model

import (
	"archive/zip"
	"bytes"
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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBinaryScript builds a shell script that behaves enough like a real binary
// for the smoke test: it prints a version string and exits 0. Padded past
// minBinarySize so the size sanity check passes.
func fakeBinaryScript(version string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "#!/bin/sh\necho 'shelltime version %s'\nexit 0\n", version)
	b.WriteString("# padding")
	b.Write(bytes.Repeat([]byte("x"), minBinarySize))
	b.WriteString("\n")
	return b.Bytes()
}

func failingBinaryScript() []byte {
	var b bytes.Buffer
	b.WriteString("#!/bin/sh\necho 'boom' >&2\nexit 3\n")
	b.WriteString("# padding")
	b.Write(bytes.Repeat([]byte("x"), minBinarySize))
	b.WriteString("\n")
	return b.Bytes()
}

// buildTestArchive returns a zip containing the named entries.
func buildTestArchive(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		hdr := &zip.FileHeader{Name: name, Method: zip.Store}
		hdr.SetMode(0o755)
		w, err := zw.CreateHeader(hdr)
		require.NoError(t, err)
		_, err = w.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// releaseServer serves an archive and a checksums.txt at the proxy paths.
type releaseServer struct {
	URL          string
	archiveHits  int
	checksumHits int
}

func newReleaseServer(t *testing.T, tag, assetName string, archive []byte, withChecksums bool) *releaseServer {
	t.Helper()
	rs := &releaseServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			rs.checksumHits++
			if !withChecksums {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			fmt.Fprintf(w, "%s  %s\n", sha256Hex(archive), assetName)
		case strings.HasSuffix(r.URL.Path, assetName):
			rs.archiveHits++
			w.Header().Set("Content-Length", fmt.Sprint(len(archive)))
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	rs.URL = srv.URL
	return rs
}

func TestApplyUpdate_ReplacesCLIAndStagesDaemon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("smoke test uses a shell script")
	}
	dir := t.TempDir()
	tag := "v9.9.9"
	assetName := "cli_Test_x86.zip"

	archive := buildTestArchive(t, map[string][]byte{
		"shelltime":        fakeBinaryScript("9.9.9"),
		"shelltime-daemon": fakeBinaryScript("9.9.9"),
	})
	srv := newReleaseServer(t, tag, assetName, archive, true)

	cliDest := filepath.Join(dir, "shelltime")
	require.NoError(t, os.WriteFile(cliDest, fakeBinaryScript("1.0.0"), 0o755))
	stagePath := filepath.Join(dir, "shelltime-daemon.next")

	res, err := ApplyUpdate(context.Background(), UpdatePlan{
		Tag:             tag,
		ArchiveName:     assetName,
		Source:          NewReleaseSource(srv.URL),
		CLIDest:         cliDest,
		DaemonStagePath: stagePath,
	})
	require.NoError(t, err)

	assert.True(t, res.Verified, "checksums.txt was available, so it must verify")
	assert.True(t, res.ReplacedCLI)
	assert.True(t, res.StagedDaemon)
	assert.False(t, res.ReplacedDaemon, "daemon must be staged, not activated")
	assert.False(t, res.UsedFallback)

	installed, err := os.ReadFile(cliDest)
	require.NoError(t, err)
	assert.Contains(t, string(installed), "9.9.9")

	staged, err := os.Stat(stagePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), staged.Mode().Perm())

	// The previous binary is preserved under .prev, never .bak.
	prev, err := os.ReadFile(cliDest + BackupSuffixUpdate)
	require.NoError(t, err)
	assert.Contains(t, string(prev), "1.0.0")
	_, err = os.Stat(cliDest + BackupSuffixLegacy)
	assert.True(t, os.IsNotExist(err))
}

// The unattended daemon path must never install a binary it could not verify.
func TestApplyUpdate_FailsClosedWithoutChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("smoke test uses a shell script")
	}
	dir := t.TempDir()
	assetName := "cli_Test_x86.zip"
	archive := buildTestArchive(t, map[string][]byte{"shelltime": fakeBinaryScript("9.9.9")})
	srv := newReleaseServer(t, "v9.9.9", assetName, archive, false /* no checksums.txt */)

	cliDest := filepath.Join(dir, "shelltime")
	require.NoError(t, os.WriteFile(cliDest, fakeBinaryScript("1.0.0"), 0o755))

	_, err := ApplyUpdate(context.Background(), UpdatePlan{
		Tag:         "v9.9.9",
		ArchiveName: assetName,
		Source:      NewReleaseSource(srv.URL),
		CLIDest:     cliDest,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no checksum available")

	// Nothing was touched.
	current, readErr := os.ReadFile(cliDest)
	require.NoError(t, readErr)
	assert.Contains(t, string(current), "1.0.0")
}

func TestApplyUpdate_AllowUnverifiedProceeds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("smoke test uses a shell script")
	}
	dir := t.TempDir()
	assetName := "cli_Test_x86.zip"
	archive := buildTestArchive(t, map[string][]byte{"shelltime": fakeBinaryScript("9.9.9")})
	srv := newReleaseServer(t, "v9.9.9", assetName, archive, false)

	cliDest := filepath.Join(dir, "shelltime")
	require.NoError(t, os.WriteFile(cliDest, fakeBinaryScript("1.0.0"), 0o755))

	res, err := ApplyUpdate(context.Background(), UpdatePlan{
		Tag:             "v9.9.9",
		ArchiveName:     assetName,
		Source:          NewReleaseSource(srv.URL),
		CLIDest:         cliDest,
		AllowUnverified: true,
	})
	require.NoError(t, err)
	assert.False(t, res.Verified)
	assert.True(t, res.ReplacedCLI)
}

// The API-provided sha and checksums.txt should agree; disagreement means
// something upstream is inconsistent and nothing should be installed.
func TestApplyUpdate_AbortsOnChecksumDisagreement(t *testing.T) {
	dir := t.TempDir()
	assetName := "cli_Test_x86.zip"
	archive := buildTestArchive(t, map[string][]byte{"shelltime": fakeBinaryScript("9.9.9")})
	srv := newReleaseServer(t, "v9.9.9", assetName, archive, true)

	cliDest := filepath.Join(dir, "shelltime")

	_, err := ApplyUpdate(context.Background(), UpdatePlan{
		Tag:         "v9.9.9",
		ArchiveName: assetName,
		Source:      NewReleaseSource(srv.URL),
		ExpectedSha: strings.Repeat("a", 64), // deliberately wrong
		CLIDest:     cliDest,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
	assert.Equal(t, 0, srv.archiveHits, "must not download when checksums disagree")
}

func TestApplyUpdate_AbortsOnCorruptDownload(t *testing.T) {
	dir := t.TempDir()
	assetName := "cli_Test_x86.zip"
	archive := buildTestArchive(t, map[string][]byte{"shelltime": fakeBinaryScript("9.9.9")})

	// Serve a checksum for the real archive but a different body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			fmt.Fprintf(w, "%s  %s\n", sha256Hex(archive), assetName)
			return
		}
		_, _ = w.Write([]byte("this is not the archive you are looking for"))
	}))
	defer srv.Close()

	cliDest := filepath.Join(dir, "shelltime")
	_, err := ApplyUpdate(context.Background(), UpdatePlan{
		Tag:         "v9.9.9",
		ArchiveName: assetName,
		// Direct source, so a checksum failure is terminal rather than
		// triggering the GitHub fallback.
		Source:  ReleaseSource{},
		CLIDest: cliDest,
	})
	require.Error(t, err)

	_, statErr := os.Stat(cliDest)
	assert.True(t, os.IsNotExist(statErr), "nothing should be installed from a corrupt download")
}

// A broken binary must never be left in place: the post-install check has to
// roll back, or the user's shell is left with a CLI that cannot run.
func TestApplyUpdate_RollsBackWhenInstalledBinaryIsBroken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("smoke test uses a shell script")
	}
	dir := t.TempDir()
	assetName := "cli_Test_x86.zip"
	archive := buildTestArchive(t, map[string][]byte{"shelltime": failingBinaryScript()})
	srv := newReleaseServer(t, "v9.9.9", assetName, archive, true)

	cliDest := filepath.Join(dir, "shelltime")
	require.NoError(t, os.WriteFile(cliDest, fakeBinaryScript("1.0.0"), 0o755))

	_, err := ApplyUpdate(context.Background(), UpdatePlan{
		Tag:         "v9.9.9",
		ArchiveName: assetName,
		Source:      NewReleaseSource(srv.URL),
		CLIDest:     cliDest,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "smoke test")

	// The working binary is still there.
	current, readErr := os.ReadFile(cliDest)
	require.NoError(t, readErr)
	assert.Contains(t, string(current), "1.0.0", "the previous working binary must survive")
}

// A user who CAN reach GitHub should not be blocked by a broken proxy.
func TestApplyUpdate_FallsBackToGitHubWhenProxyFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("smoke test uses a shell script")
	}
	dir := t.TempDir()
	tag := "v9.9.9"
	assetName := "cli_Test_x86.zip"
	archive := buildTestArchive(t, map[string][]byte{"shelltime": fakeBinaryScript("9.9.9")})

	// "GitHub" serves everything; the proxy 500s on the archive only.
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			fmt.Fprintf(w, "%s  %s\n", sha256Hex(archive), assetName)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(archive)))
		_, _ = w.Write(archive)
	}))
	defer github.Close()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "checksums.txt") {
			fmt.Fprintf(w, "%s  %s\n", sha256Hex(archive), assetName)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer proxy.Close()

	prevRelease := githubReleaseBaseURL
	githubReleaseBaseURL = github.URL
	defer func() { githubReleaseBaseURL = prevRelease }()

	cliDest := filepath.Join(dir, "shelltime")
	res, err := ApplyUpdate(context.Background(), UpdatePlan{
		Tag:         tag,
		ArchiveName: assetName,
		Source:      NewReleaseSource(proxy.URL),
		CLIDest:     cliDest,
	})
	require.NoError(t, err)
	assert.True(t, res.UsedFallback, "should report that it fell back to GitHub")
	assert.True(t, res.ReplacedCLI)
}

func TestApplyUpdate_RequiresTag(t *testing.T) {
	_, err := ApplyUpdate(context.Background(), UpdatePlan{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tag")
}
