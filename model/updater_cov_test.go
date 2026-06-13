package model

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractBinaries_UnsupportedFormat covers the format-dispatch error branch.
func TestExtractBinaries_UnsupportedFormat(t *testing.T) {
	_, err := ExtractBinaries(filepath.Join(t.TempDir(), "release.rar"), t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported archive format")
}

// TestExtractZipBinaries_SkipsNonAllowedEntries builds a zip containing both a
// disallowed file and an allowed binary, asserting only the allowed one is
// extracted (covers the !allowedArchiveBinaries continue branch).
func TestExtractZipBinaries_SkipsNonAllowedEntries(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "release.zip")
	zf, err := os.Create(archivePath)
	require.NoError(t, err)
	zw := zip.NewWriter(zf)

	// disallowed entry
	w1, err := zw.Create("README.md")
	require.NoError(t, err)
	_, _ = w1.Write([]byte("docs"))
	// allowed binary
	w2, err := zw.Create("shelltime")
	require.NoError(t, err)
	_, _ = w2.Write([]byte("BIN"))

	require.NoError(t, zw.Close())
	require.NoError(t, zf.Close())

	dest := t.TempDir()
	out, err := ExtractBinaries(archivePath, dest)
	require.NoError(t, err)
	require.Len(t, out, 1)
	p, ok := out["shelltime"]
	require.True(t, ok)
	content, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "BIN", string(content))
}

// TestExtractTarGzBinaries_SkipsDirsAndNonAllowed builds a tar.gz with a
// directory entry, a disallowed file and an allowed binary; only the binary is
// extracted (covers the Typeflag != TypeReg and !allowed continue branches).
func TestExtractTarGzBinaries_SkipsDirsAndNonAllowed(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "release.tar.gz")
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// directory entry -> skipped
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "subdir/", Typeflag: tar.TypeDir, Mode: 0o755}))
	// disallowed regular file -> skipped
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "LICENSE", Typeflag: tar.TypeReg, Size: 3, Mode: 0o644}))
	_, _ = tw.Write([]byte("mit"))
	// allowed daemon binary -> extracted
	body := []byte("DAEMON")
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "shelltime-daemon", Typeflag: tar.TypeReg, Size: int64(len(body)), Mode: 0o755}))
	_, _ = tw.Write(body)

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	require.NoError(t, f.Close())

	dest := t.TempDir()
	out, err := ExtractBinaries(archivePath, dest)
	require.NoError(t, err)
	require.Len(t, out, 1)
	p, ok := out["shelltime-daemon"]
	require.True(t, ok)
	content, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "DAEMON", string(content))
}

// TestExtractBinaries_CorruptZip covers the zip.OpenReader error branch.
func TestExtractBinaries_CorruptZip(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "release.zip")
	require.NoError(t, os.WriteFile(bad, []byte("not a real zip"), 0o644))
	_, err := ExtractBinaries(bad, t.TempDir())
	require.Error(t, err)
}

// TestExtractBinaries_CorruptTarGz covers the gzip.NewReader error branch.
func TestExtractBinaries_CorruptTarGz(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "release.tar.gz")
	require.NoError(t, os.WriteFile(bad, []byte("not gzip"), 0o644))
	_, err := ExtractBinaries(bad, t.TempDir())
	require.Error(t, err)
}

// TestWriteBinary_RoundTrip exercises writeBinary directly with a small reader.
func TestWriteBinary_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "bin")
	require.NoError(t, writeBinary(target, strings.NewReader("payload")))
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(got))
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

// TestReplaceBinary_RestoreOnMoveFailure covers ReplaceBinary's failure-recovery
// branch: when moveFile fails (src missing), the prior binary is restored from
// the .bak and an error is returned.
func TestReplaceBinary_RestoreOnMoveFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shelltime")
	require.NoError(t, os.WriteFile(dest, []byte("ORIGINAL"), 0o755))

	// src does not exist -> moveFile fails after dest was renamed to .bak.
	missingSrc := filepath.Join(dir, "missing-src")
	err := ReplaceBinary(missingSrc, dest)
	require.Error(t, err)

	// The original binary must have been restored to dest.
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(got), "prior binary restored after failed move")
}
