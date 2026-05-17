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

func TestBuildArchiveName(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{"linux amd64", "linux", "amd64", "cli_Linux_x86_64.tar.gz", false},
		{"linux arm64", "linux", "arm64", "cli_Linux_arm64.tar.gz", false},
		{"darwin amd64", "darwin", "amd64", "cli_Darwin_x86_64.zip", false},
		{"darwin arm64", "darwin", "arm64", "cli_Darwin_arm64.zip", false},
		{"windows amd64", "windows", "amd64", "cli_Windows_x86_64.zip", false},
		{"windows arm64", "windows", "arm64", "cli_Windows_arm64.zip", false},
		{"linux 386", "linux", "386", "cli_Linux_i386.tar.gz", false},
		{"unsupported os", "freebsd", "amd64", "", true},
		{"unsupported arch", "linux", "mips", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildArchiveName(tt.goos, tt.goarch)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildDownloadURL(t *testing.T) {
	got := BuildDownloadURL("v1.2.3", "cli_Linux_x86_64.tar.gz")
	assert.Equal(t, "https://github.com/shelltime/cli/releases/download/v1.2.3/cli_Linux_x86_64.tar.gz", got)
}

func TestBuildChecksumsURL(t *testing.T) {
	got := BuildChecksumsURL("v1.2.3")
	assert.Equal(t, "https://github.com/shelltime/cli/releases/download/v1.2.3/checksums.txt", got)
}

func TestParseChecksumLine(t *testing.T) {
	content := strings.Join([]string{
		"abc123  some_other_file.tar.gz",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  cli_Linux_x86_64.tar.gz",
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  cli_Darwin_arm64.zip",
		"",
	}, "\n")

	t.Run("found", func(t *testing.T) {
		sum, ok, err := parseChecksumLine(content, "cli_Linux_x86_64.tar.gz")
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", sum)
	})

	t.Run("missing", func(t *testing.T) {
		sum, ok, err := parseChecksumLine(content, "cli_Windows_arm64.zip")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, sum)
	})

	t.Run("malformed sha", func(t *testing.T) {
		_, _, err := parseChecksumLine("abc123  cli_Linux_x86_64.tar.gz", "cli_Linux_x86_64.tar.gz")
		assert.Error(t, err)
	})
}

func TestSafeExtractPath(t *testing.T) {
	tests := []struct {
		name      string
		destDir   string
		entryName string
		wantErr   bool
	}{
		{"clean basename", "/tmp/foo", "shelltime", false},
		{"path traversal stripped by basename", "/tmp/foo", "../../etc/passwd", false},
		{"absolute stripped by basename", "/tmp/foo", "/etc/passwd", false},
		{"nested name", "/tmp/foo", "bin/shelltime", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeExtractPath(tt.destDir, tt.entryName)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(got, "/tmp/foo"))
			assert.Equal(t, filepath.Base(tt.entryName), filepath.Base(got))
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"v0.94.5", "0.94.5"},
		{"0.94.5", "0.94.5"},
		{"  v1.0.0  ", "1.0.0"},
		{"", ""},
		{"v", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeVersion(tt.in))
		})
	}
}

func TestDetectInstallKind(t *testing.T) {
	base := GetBaseStoragePath()
	tests := []struct {
		name string
		path string
		want InstallKind
	}{
		{"curl install", filepath.Join(base, "bin", "shelltime"), InstallKindCurl},
		{"homebrew apple silicon", "/opt/homebrew/bin/shelltime", InstallKindHomebrew},
		{"homebrew cellar", "/usr/local/Cellar/shelltime/0.1.0/bin/shelltime", InstallKindHomebrew},
		{"linuxbrew", "/home/linuxbrew/.linuxbrew/bin/shelltime", InstallKindHomebrew},
		{"random location", "/usr/bin/shelltime", InstallKindUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DetectInstallKind(tt.path))
		})
	}
}

func TestExtractBinariesZip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "release.zip")

	zf, err := os.Create(archivePath)
	require.NoError(t, err)
	zw := zip.NewWriter(zf)

	cli, err := zw.Create("shelltime")
	require.NoError(t, err)
	_, err = cli.Write([]byte("CLI_BINARY"))
	require.NoError(t, err)

	daemon, err := zw.Create("shelltime-daemon")
	require.NoError(t, err)
	_, err = daemon.Write([]byte("DAEMON_BINARY"))
	require.NoError(t, err)

	junk, err := zw.Create("README.md")
	require.NoError(t, err)
	_, err = junk.Write([]byte("ignored"))
	require.NoError(t, err)

	require.NoError(t, zw.Close())
	require.NoError(t, zf.Close())

	dest := filepath.Join(tmp, "out")
	require.NoError(t, os.MkdirAll(dest, 0o755))

	got, err := ExtractBinaries(archivePath, dest)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Contains(t, got, "shelltime")
	assert.Contains(t, got, "shelltime-daemon")

	body, err := os.ReadFile(got["shelltime"])
	require.NoError(t, err)
	assert.Equal(t, "CLI_BINARY", string(body))
}

func TestExtractBinariesTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "release.tar.gz")

	tf, err := os.Create(archivePath)
	require.NoError(t, err)
	gw := gzip.NewWriter(tf)
	tw := tar.NewWriter(gw)

	writeEntry := func(name, body string) {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}

	writeEntry("shelltime", "CLI_TAR")
	writeEntry("shelltime-daemon", "DAEMON_TAR")
	writeEntry("LICENSE", "ignored")

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	require.NoError(t, tf.Close())

	dest := filepath.Join(tmp, "out")
	require.NoError(t, os.MkdirAll(dest, 0o755))

	got, err := ExtractBinaries(archivePath, dest)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	body, err := os.ReadFile(got["shelltime"])
	require.NoError(t, err)
	assert.Equal(t, "CLI_TAR", string(body))
}

func TestReplaceBinary(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "shelltime")
	require.NoError(t, os.WriteFile(dest, []byte("OLD"), 0o755))

	src := filepath.Join(tmp, "src", "shelltime")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte("NEW"), 0o755))

	require.NoError(t, ReplaceBinary(src, dest))

	body, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "NEW", string(body))

	body, err = os.ReadFile(dest + ".bak")
	require.NoError(t, err)
	assert.Equal(t, "OLD", string(body))
}

func TestReplaceBinaryNoExisting(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "shelltime")

	src := filepath.Join(tmp, "src", "shelltime")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte("NEW"), 0o755))

	require.NoError(t, ReplaceBinary(src, dest))

	body, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "NEW", string(body))

	_, err = os.Stat(dest + ".bak")
	assert.True(t, os.IsNotExist(err))
}
