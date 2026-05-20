package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// daemonFetchGOOS is a swappable indirection over runtime.GOOS so tests can
// exercise the Windows-aborts branch on Linux/Darwin runners.
var daemonFetchGOOS = runtime.GOOS

// EnsureDaemonBinary returns the path to a usable shelltime-daemon binary,
// downloading it from GitHub releases into the curl-installer location
// (~/.shelltime/bin/shelltime-daemon) when no existing binary is found.
//
// cliBinPath is the resolved path of the running CLI; it is consulted only
// to refuse auto-download for Homebrew installs (those should use
// `brew reinstall`). cliVersion is the CLI's own version (typically
// commands.commitID); when empty or "dev" we fall back to the latest
// GitHub release tag.
func EnsureDaemonBinary(ctx context.Context, cliBinPath, cliVersion string) (string, error) {
	if p, err := ResolveDaemonBinaryPath(); err == nil {
		return p, nil
	}

	if daemonFetchGOOS == "windows" {
		return "", fmt.Errorf("shelltime-daemon is not built for Windows; please use WSL or run the CLI without the daemon")
	}

	if cliBinPath != "" && DetectInstallKind(cliBinPath) == InstallKindHomebrew {
		return "", fmt.Errorf("shelltime-daemon missing for Homebrew install; run: brew reinstall shelltime/tap/shelltime")
	}

	tag := normalizeDaemonTag(cliVersion)
	if tag == "" {
		latest, err := FetchLatestVersion(ctx)
		if err != nil {
			return "", fmt.Errorf("cannot determine release tag (no CLI version, latest lookup failed): %w", err)
		}
		tag = latest
	}

	archiveName, err := BuildArchiveName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}

	destPath, err := fetchDaemonToCurlPath(ctx, tag, archiveName)
	if err == nil {
		return destPath, nil
	}

	// Fall back once to latest if the tagged release 404'd (deleted/yanked).
	if strings.Contains(err.Error(), "status 404") {
		latest, lerr := FetchLatestVersion(ctx)
		if lerr == nil && latest != "" && latest != tag {
			if destPath, ferr := fetchDaemonToCurlPath(ctx, latest, archiveName); ferr == nil {
				return destPath, nil
			}
		}
	}
	return "", err
}

// fetchDaemonToCurlPath downloads, verifies, extracts, and installs the
// shelltime-daemon binary from release `tag` to GetCurlInstallerDaemonPath().
func fetchDaemonToCurlPath(ctx context.Context, tag, archiveName string) (string, error) {
	downloadURL := BuildDownloadURL(tag, archiveName)

	sum, _, err := FetchChecksum(ctx, tag, archiveName)
	if err != nil {
		return "", fmt.Errorf("fetch checksum: %w", err)
	}
	// Empty sum (404 / no entry for this archive) is legitimately absent —
	// proceed without verification. Non-nil errors (5xx, network, MITM) MUST
	// propagate so an attacker can't silently downgrade us to an unverified
	// download.

	tmpDir, err := os.MkdirTemp("", "shelltime-daemon-fetch-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := DownloadAndVerify(ctx, downloadURL, sum, archivePath); err != nil {
		return "", fmt.Errorf("download daemon archive: %w", err)
	}

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", err
	}
	binaries, err := ExtractBinaries(archivePath, extractDir)
	if err != nil {
		return "", fmt.Errorf("extract archive: %w", err)
	}
	daemonSrc, ok := binaries["shelltime-daemon"]
	if !ok {
		return "", fmt.Errorf("archive %s did not contain shelltime-daemon", archiveName)
	}

	destPath := GetCurlInstallerDaemonPath()
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("create bin dir: %w", err)
	}
	if err := ReplaceBinary(daemonSrc, destPath); err != nil {
		return "", fmt.Errorf("install daemon binary: %w", err)
	}
	return destPath, nil
}

// normalizeDaemonTag returns "" for empty/"dev" inputs (signaling the caller
// to fetch the latest tag), otherwise ensures the result has a leading "v"
// to match goreleaser's tag template.
func normalizeDaemonTag(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}
