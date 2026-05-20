package model

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	// GitHub repo for ShellTime CLI releases.
	githubReleasesOwner = "shelltime"
	githubReleasesRepo  = "cli"

	// Goreleaser ProjectName for release archive naming (cli_<OS>_<ARCH>.<ext>).
	releaseArchivePrefix = "cli"

	// Max bytes accepted from an archive entry to defeat zip-bombs.
	maxArchiveEntrySize = 200 * 1024 * 1024

	// HTTP timeouts.
	updaterAPITimeout      = 15 * time.Second
	updaterDownloadTimeout = 5 * time.Minute
)

// Base URLs for GitHub. Exposed as vars so tests can point them at an
// httptest.Server.
var (
	githubAPIBaseURL     = "https://api.github.com"
	githubReleaseBaseURL = "https://github.com"
)

// Binary names extracted from release archives.
var allowedArchiveBinaries = map[string]bool{
	"shelltime":            true,
	"shelltime-daemon":     true,
	"shelltime.exe":        true,
	"shelltime-daemon.exe": true,
}

// LatestRelease is the subset of the GitHub releases API response we use.
type LatestRelease struct {
	TagName string `json:"tag_name"`
}

func newUpdaterHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

func updaterUserAgent() string {
	v := commitID
	if v == "" {
		v = "dev"
	}
	return "shelltimeCLI@" + v
}

// FetchLatestVersion calls the GitHub API for the latest stable release tag.
func FetchLatestVersion(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIBaseURL, githubReleasesOwner, githubReleasesRepo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", updaterUserAgent())
	req.Header.Set("Accept", "application/vnd.github+json")

	client := newUpdaterHTTPClient(updaterAPITimeout)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var rel LatestRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("decode github api response: %w", err)
	}
	if rel.TagName == "" {
		return "", errors.New("github api returned empty tag_name")
	}
	return rel.TagName, nil
}

// BuildArchiveName returns the release archive filename for the given platform,
// matching the goreleaser name_template (e.g. cli_Darwin_x86_64.zip).
func BuildArchiveName(goos, goarch string) (string, error) {
	var osPart string
	switch goos {
	case "darwin":
		osPart = "Darwin"
	case "linux":
		osPart = "Linux"
	case "windows":
		osPart = "Windows"
	default:
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}

	var archPart string
	switch goarch {
	case "amd64":
		archPart = "x86_64"
	case "arm64":
		archPart = "arm64"
	case "386":
		archPart = "i386"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	ext := "tar.gz"
	if goos == "darwin" || goos == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf("%s_%s_%s.%s", releaseArchivePrefix, osPart, archPart, ext), nil
}

// BuildDownloadURL returns the direct release-asset URL for a specific tag.
func BuildDownloadURL(tag, archiveName string) string {
	return fmt.Sprintf(
		"%s/%s/%s/releases/download/%s/%s",
		githubReleaseBaseURL, githubReleasesOwner, githubReleasesRepo, tag, archiveName,
	)
}

// BuildChecksumsURL returns the checksums.txt URL for a specific tag.
func BuildChecksumsURL(tag string) string {
	return fmt.Sprintf(
		"%s/%s/%s/releases/download/%s/checksums.txt",
		githubReleaseBaseURL, githubReleasesOwner, githubReleasesRepo, tag,
	)
}

// FetchChecksum returns the expected SHA256 for archiveName. The bool reports
// whether a checksum was found; callers may proceed without verification if false.
func FetchChecksum(ctx context.Context, tag, archiveName string) (string, bool, error) {
	url := BuildChecksumsURL(tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", updaterUserAgent())

	client := newUpdaterHTTPClient(updaterAPITimeout)
	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("download checksums.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("checksums.txt returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return "", false, fmt.Errorf("read checksums.txt: %w", err)
	}
	return parseChecksumLine(string(body), archiveName)
}

// parseChecksumLine finds the SHA256 for archiveName in goreleaser's checksums.txt
// format: "<sha256>  <filename>".
func parseChecksumLine(content, archiveName string) (string, bool, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[1] == archiveName {
			if len(fields[0]) != 64 {
				return "", false, fmt.Errorf("malformed sha256 for %s: %q", archiveName, fields[0])
			}
			return strings.ToLower(fields[0]), true, nil
		}
	}
	return "", false, nil
}

// DownloadAndVerify streams url to destPath while hashing. If expectedSha256 is
// non-empty, the download fails unless the computed digest matches.
func DownloadAndVerify(ctx context.Context, url, expectedSha256, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", updaterUserAgent())

	client := newUpdaterHTTPClient(updaterDownloadTimeout)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned status %d", url, resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	hasher := sha256.New()
	if _, err := io.Copy(out, io.TeeReader(resp.Body, hasher)); err != nil {
		return fmt.Errorf("write archive: %w", err)
	}

	if expectedSha256 != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(got, expectedSha256) {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSha256, got)
		}
	}
	return nil
}

// safeExtractPath joins destDir and entryName, rejecting any result that escapes
// destDir (defends against zip-slip / tar path traversal).
func safeExtractPath(destDir, entryName string) (string, error) {
	cleanDest := filepath.Clean(destDir)
	target := filepath.Join(cleanDest, filepath.Base(entryName))
	if !strings.HasPrefix(target, cleanDest+string(filepath.Separator)) && target != cleanDest {
		return "", fmt.Errorf("archive entry escapes destination: %q", entryName)
	}
	return target, nil
}

// ExtractBinaries unpacks the archive into tmpDir, returning a map from binary
// basename (without .exe) to the extracted file path. Only entries matching the
// allowed binary names are extracted.
func ExtractBinaries(archivePath, tmpDir string) (map[string]string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZipBinaries(archivePath, tmpDir)
	}
	if strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		return extractTarGzBinaries(archivePath, tmpDir)
	}
	return nil, fmt.Errorf("unsupported archive format: %s", archivePath)
}

func extractZipBinaries(archivePath, tmpDir string) (map[string]string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	out := map[string]string{}
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if !allowedArchiveBinaries[base] {
			continue
		}
		target, err := safeExtractPath(tmpDir, base)
		if err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		if err := writeBinary(target, rc); err != nil {
			rc.Close()
			return nil, err
		}
		rc.Close()
		out[stripExe(base)] = target
	}
	return out, nil
}

func extractTarGzBinaries(archivePath, tmpDir string) (map[string]string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	out := map[string]string{}
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		base := filepath.Base(hdr.Name)
		if !allowedArchiveBinaries[base] {
			continue
		}
		target, err := safeExtractPath(tmpDir, base)
		if err != nil {
			return nil, err
		}
		if err := writeBinary(target, tr); err != nil {
			return nil, err
		}
		out[stripExe(base)] = target
	}
	return out, nil
}

func writeBinary(target string, src io.Reader) error {
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, io.LimitReader(src, maxArchiveEntrySize+1)); err != nil {
		return err
	}
	info, err := dst.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxArchiveEntrySize {
		return fmt.Errorf("archive entry %s exceeds max size %d", target, maxArchiveEntrySize)
	}
	return nil
}

func stripExe(name string) string {
	return strings.TrimSuffix(name, ".exe")
}

// ReplaceBinary swaps a freshly-downloaded binary into destPath, renaming any
// existing destPath to destPath+".bak" (overwriting a previous .bak). On Unix
// this is safe even while the binary is running because the kernel keeps the
// old inode alive for the current process.
func ReplaceBinary(srcPath, destPath string) error {
	bak := destPath + ".bak"
	_ = os.Remove(bak)
	if _, err := os.Stat(destPath); err == nil {
		if err := os.Rename(destPath, bak); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", destPath, bak, err)
		}
	}
	if err := moveFile(srcPath, destPath); err != nil {
		// Try to restore .bak on failure so we don't leave the user without a binary.
		_ = os.Rename(bak, destPath)
		return err
	}
	if err := os.Chmod(destPath, 0o755); err != nil {
		return err
	}
	return nil
}

// moveFile renames src to dst, falling back to copy+remove when crossing
// filesystems (e.g. /tmp to $HOME on Linux with separate mounts).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// NormalizeVersion strips a leading "v" so "v0.94.5" and "0.94.5" compare equal.
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// ResolveCLIBinaryPath returns the real (symlink-resolved) path of the running
// CLI binary.
func ResolveCLIBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return real, nil
}

// InstallKind describes how the running CLI binary appears to be installed.
type InstallKind int

const (
	InstallKindUnknown InstallKind = iota
	InstallKindHomebrew
	InstallKindCurl
)

// DetectInstallKind classifies binPath as a Homebrew install, a curl-installer
// install ($HOME/.shelltime/bin), or unknown.
func DetectInstallKind(binPath string) InstallKind {
	clean := filepath.Clean(binPath)
	if strings.Contains(clean, string(filepath.Separator)+"Cellar"+string(filepath.Separator)) ||
		strings.HasPrefix(clean, "/opt/homebrew/") ||
		strings.HasPrefix(clean, "/home/linuxbrew/.linuxbrew/") {
		return InstallKindHomebrew
	}
	expected := filepath.Clean(filepath.Join(GetBaseStoragePath(), "bin"))
	if strings.HasPrefix(clean, expected+string(filepath.Separator)) {
		return InstallKindCurl
	}
	return InstallKindUnknown
}

// CurrentPlatform returns the goos/goarch pair, exposed for tests and logging.
func CurrentPlatform() (string, string) {
	return runtime.GOOS, runtime.GOARCH
}
