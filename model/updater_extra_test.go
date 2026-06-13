package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentPlatform(t *testing.T) {
	goos, goarch := CurrentPlatform()
	assert.Equal(t, runtime.GOOS, goos)
	assert.Equal(t, runtime.GOARCH, goarch)
}

func TestResolveCLIBinaryPath(t *testing.T) {
	got, err := ResolveCLIBinaryPath()
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	assert.True(t, filepath.IsAbs(got), "resolved CLI path should be absolute")
}

func TestFetchLatestVersion(t *testing.T) {
	t.Run("happy path returns tag", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.Path, "/releases/latest")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
		}))
		defer server.Close()

		orig := githubAPIBaseURL
		githubAPIBaseURL = server.URL
		t.Cleanup(func() { githubAPIBaseURL = orig })

		tag, err := FetchLatestVersion(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "v1.2.3", tag)
	})

	t.Run("empty tag_name returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tag_name":""}`))
		}))
		defer server.Close()

		orig := githubAPIBaseURL
		githubAPIBaseURL = server.URL
		t.Cleanup(func() { githubAPIBaseURL = orig })

		_, err := FetchLatestVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty tag_name")
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		orig := githubAPIBaseURL
		githubAPIBaseURL = server.URL
		t.Cleanup(func() { githubAPIBaseURL = orig })

		_, err := FetchLatestVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
	})
}

func TestFetchChecksum(t *testing.T) {
	const archive = "cli_Linux_x86_64.tar.gz"
	validSum := hex.EncodeToString(sha256.New().Sum(nil)) // 64 hex chars

	t.Run("found returns lowercased sha", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.Path, "checksums.txt")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(validSum + "  " + archive + "\n"))
		}))
		defer server.Close()

		orig := githubReleaseBaseURL
		githubReleaseBaseURL = server.URL
		t.Cleanup(func() { githubReleaseBaseURL = orig })

		sum, ok, err := FetchChecksum(context.Background(), "v1.0.0", archive)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, validSum, sum)
	})

	t.Run("not found in file returns ok=false", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(validSum + "  some_other_file.zip\n"))
		}))
		defer server.Close()

		orig := githubReleaseBaseURL
		githubReleaseBaseURL = server.URL
		t.Cleanup(func() { githubReleaseBaseURL = orig })

		_, ok, err := FetchChecksum(context.Background(), "v1.0.0", archive)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("404 means no checksum without error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		orig := githubReleaseBaseURL
		githubReleaseBaseURL = server.URL
		t.Cleanup(func() { githubReleaseBaseURL = orig })

		_, ok, err := FetchChecksum(context.Background(), "v1.0.0", archive)
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestDownloadAndVerify(t *testing.T) {
	payload := []byte("the release archive bytes")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	t.Run("downloads and verifies matching checksum", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "archive.bin")
		err := DownloadAndVerify(context.Background(), server.URL, hexSum, dest)
		require.NoError(t, err)
		got, err := os.ReadFile(dest)
		require.NoError(t, err)
		assert.Equal(t, payload, got)
	})

	t.Run("checksum mismatch errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "archive.bin")
		err := DownloadAndVerify(context.Background(), server.URL, "deadbeef", dest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "checksum mismatch")
	})

	t.Run("empty expected checksum skips verification", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "archive.bin")
		err := DownloadAndVerify(context.Background(), server.URL, "", dest)
		require.NoError(t, err)
	})

	t.Run("non-200 status errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		dest := filepath.Join(t.TempDir(), "archive.bin")
		err := DownloadAndVerify(context.Background(), server.URL, "", dest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 404")
	})
}

func TestMoveFile(t *testing.T) {
	t.Run("same-dir rename", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src")
		dst := filepath.Join(dir, "dst")
		require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))

		require.NoError(t, moveFile(src, dst))
		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(got))
		_, statErr := os.Stat(src)
		assert.True(t, os.IsNotExist(statErr), "source removed after move")
	})

	t.Run("missing source errors", func(t *testing.T) {
		dir := t.TempDir()
		err := moveFile(filepath.Join(dir, "nope"), filepath.Join(dir, "dst"))
		require.Error(t, err)
	})
}
