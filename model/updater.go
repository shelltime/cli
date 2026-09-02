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
	"strconv"
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

// FetchChecksum returns the expected SHA256 for archiveName, fetched from
// GitHub directly. The bool reports whether a checksum was found; callers may
// proceed without verification if false.
func FetchChecksum(ctx context.Context, tag, archiveName string) (string, bool, error) {
	return FetchChecksumFrom(ctx, ReleaseSource{}, tag, archiveName)
}

// FetchChecksumFrom returns the expected SHA256 for archiveName, fetched through
// the given source (proxy or GitHub direct).
func FetchChecksumFrom(ctx context.Context, src ReleaseSource, tag, archiveName string) (string, bool, error) {
	return fetchChecksumURL(ctx, src.ChecksumsURL(tag), archiveName)
}

func fetchChecksumURL(ctx context.Context, url, archiveName string) (string, bool, error) {
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
	written, err := io.Copy(out, io.TeeReader(resp.Body, hasher))
	if err != nil {
		return fmt.Errorf("write archive: %w", err)
	}

	// A proxy (or a flaky link) can close the stream early. Without a checksum
	// that truncation would otherwise sail through to the extract step.
	if resp.ContentLength >= 0 && written != resp.ContentLength {
		return fmt.Errorf("truncated download %s: got %d bytes, expected %d", url, written, resp.ContentLength)
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

// BackupSuffixLegacy is the historical backup suffix written by ReplaceBinary.
//
// WARNING: commands/daemon.install.go treats "<daemon>.bak" as "a NEWER daemon
// that should be restored", which is the opposite of what ReplaceBinary means by
// it. Any code path that replaces the daemon binary and then runs
// `daemon install`/`daemon reinstall` MUST use BackupSuffixUpdate instead, or the
// install step will restore the binary that was just replaced.
const BackupSuffixLegacy = ".bak"

// BackupSuffixUpdate is the backup suffix used by the self-update paths. It is
// deliberately distinct from BackupSuffixLegacy so the daemon installer's
// ".bak means restore me" recovery branch never fires on an update backup.
const BackupSuffixUpdate = ".prev"

// ReplaceBinary swaps a freshly-downloaded binary into destPath, keeping the
// previous binary at destPath+".bak".
//
// Prefer ReplaceBinaryWithBackupSuffix with BackupSuffixUpdate for the daemon
// binary; see BackupSuffixLegacy for why.
func ReplaceBinary(srcPath, destPath string) error {
	return ReplaceBinaryWithBackupSuffix(srcPath, destPath, BackupSuffixLegacy)
}

// ReplaceBinaryWithBackupSuffix swaps srcPath into destPath, preserving the
// previous binary at destPath+suffix (overwriting any previous backup).
//
// On Unix the swap is atomic: the old binary is *copied* to the backup, the new
// binary is staged alongside destPath, and a single rename(2) puts it in place.
// destPath therefore never disappears — which matters because the shell hook
// execs `shelltime` by name on every command, and a hook landing in a gap would
// print "command not found". Replacing a running binary is safe because the
// kernel keeps the old inode alive for processes that already opened it.
//
// Windows cannot rename over a running .exe, so it keeps the historical
// move-the-old-one-away-first order.
func ReplaceBinaryWithBackupSuffix(srcPath, destPath, suffix string) error {
	if suffix == "" {
		suffix = BackupSuffixUpdate
	}
	backup := destPath + suffix

	if runtime.GOOS == "windows" {
		return replaceBinaryViaRenameAway(srcPath, destPath, backup)
	}
	return replaceBinaryAtomic(srcPath, destPath, backup)
}

// replaceBinaryAtomic never leaves destPath absent. Used on all Unix platforms.
func replaceBinaryAtomic(srcPath, destPath, backup string) error {
	if _, err := os.Stat(destPath); err == nil {
		_ = os.Remove(backup)
		if err := copyFile(destPath, backup); err != nil {
			return fmt.Errorf("back up %s -> %s: %w", destPath, backup, err)
		}
	}

	// Stage in the destination directory so the final rename cannot cross a
	// filesystem boundary (rename(2) fails with EXDEV across mounts).
	staged := destPath + ".new"
	_ = os.Remove(staged)
	if err := copyFile(srcPath, staged); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("stage %s -> %s: %w", srcPath, staged, err)
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		_ = os.Remove(staged)
		return err
	}
	if err := os.Rename(staged, destPath); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("activate %s -> %s: %w", staged, destPath, err)
	}
	_ = os.Remove(srcPath)
	return nil
}

// replaceBinaryViaRenameAway is the Windows path: a running .exe cannot be
// renamed over, but it can be renamed away.
func replaceBinaryViaRenameAway(srcPath, destPath, backup string) error {
	_ = os.Remove(backup)
	if _, err := os.Stat(destPath); err == nil {
		if err := os.Rename(destPath, backup); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", destPath, backup, err)
		}
	}
	if err := moveFile(srcPath, destPath); err != nil {
		// Try to restore the backup so we don't leave the user without a binary.
		_ = os.Rename(backup, destPath)
		return err
	}
	return os.Chmod(destPath, 0o755)
}

// RestoreBinaryBackup puts the backup at destPath+suffix back at destPath. Used
// to roll back when a freshly-installed binary fails its post-swap smoke test.
func RestoreBinaryBackup(destPath, suffix string) error {
	if suffix == "" {
		suffix = BackupSuffixUpdate
	}
	backup := destPath + suffix
	if _, err := os.Stat(backup); err != nil {
		return fmt.Errorf("no backup at %s: %w", backup, err)
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(destPath)
		return os.Rename(backup, destPath)
	}
	return replaceBinaryAtomic(backup, destPath, destPath+".rollback")
}

// copyFile copies src to dst with mode 0755, truncating dst if it exists.
func copyFile(src, dst string) error {
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
	return out.Close()
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

// CompareVersions compares two dotted numeric versions, ignoring a leading "v"
// and any pre-release suffix ("1.2.3-rc1" compares as "1.2.3"). It returns -1 if
// a < b, 0 if equal, and 1 if a > b. Non-numeric or missing segments count as 0,
// so it never panics on malformed input from the server.
//
// The unattended update path uses this to guarantee it only ever moves forward:
// a server bug that reports an old tag must not downgrade the user.
func CompareVersions(a, b string) int {
	as := versionSegments(a)
	bs := versionSegments(b)
	n := max(len(as), len(bs))
	for i := range n {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionSegments(v string) []int {
	v = NormalizeVersion(v)
	// Drop pre-release/build metadata: "1.2.3-rc1+deadbeef" -> "1.2.3".
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
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
	sep := string(filepath.Separator)
	// goreleaser publishes a Cask, so an EvalSymlinks'd path lands in
	// .../Caskroom/... — /opt/homebrew/Caskroom on Apple Silicon (already matched
	// by the prefix below) but /usr/local/Caskroom on Intel, which matches
	// neither the Cellar nor the prefix checks.
	if strings.Contains(clean, sep+"Cellar"+sep) ||
		strings.Contains(clean, sep+"Caskroom"+sep) ||
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
