package model

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// smokeTestTimeout bounds how long we wait for a freshly-downloaded binary to
// print its version. A var so tests can shrink it.
var smokeTestTimeout = 10 * time.Second

// minBinarySize is a crude sanity floor. A real binary is tens of MB; anything
// this small is a truncated download or an HTML error page.
const minBinarySize = 1 << 20

// UpdatePlan describes one update application.
type UpdatePlan struct {
	Tag         string
	ArchiveName string
	Source      ReleaseSource
	// ExpectedSha comes from the server's GraphQL response. Empty means "fetch
	// checksums.txt through Source".
	ExpectedSha string

	// CLIDest, if set, is where the shelltime binary is activated.
	CLIDest string
	// DaemonDest, if set, is where the daemon binary is activated immediately.
	DaemonDest string
	// DaemonStagePath, if set, is where the daemon binary is parked WITHOUT
	// being activated. The daemon uses this so it never rewrites the binary it
	// is currently running; the CLI activates it later.
	DaemonStagePath string

	// AllowUnverified permits proceeding with no checksum. Only the interactive
	// `shelltime update` sets this; the unattended daemon path must fail closed.
	AllowUnverified bool
	// BackupSuffix defaults to BackupSuffixUpdate.
	BackupSuffix string
}

// UpdateResult reports what an ApplyUpdate call actually did.
type UpdateResult struct {
	Tag            string
	Verified       bool
	ReplacedCLI    bool
	ReplacedDaemon bool
	StagedDaemon   bool
	// UsedFallback is true when the proxy failed and GitHub-direct succeeded.
	UsedFallback bool
}

// ApplyUpdate downloads, verifies, and installs a release. It is the single
// place binaries are swapped, so every safety check lives here rather than
// being duplicated across the interactive and unattended callers.
func ApplyUpdate(ctx context.Context, plan UpdatePlan) (UpdateResult, error) {
	res := UpdateResult{Tag: plan.Tag}

	if plan.Tag == "" {
		return res, errors.New("update plan has no tag")
	}
	if plan.ArchiveName == "" {
		name, err := CurrentPlatformArchiveName()
		if err != nil {
			return res, err
		}
		plan.ArchiveName = name
	}
	if plan.BackupSuffix == "" {
		plan.BackupSuffix = BackupSuffixUpdate
	}

	sha, err := resolveExpectedChecksum(ctx, plan)
	if err != nil {
		return res, err
	}
	if sha == "" && !plan.AllowUnverified {
		return res, fmt.Errorf(
			"no checksum available for %s; refusing to install an unverified binary", plan.ArchiveName)
	}
	res.Verified = sha != ""

	tmpDir, err := os.MkdirTemp("", "shelltime-update-*")
	if err != nil {
		return res, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, plan.ArchiveName)
	usedFallback, err := downloadWithFallback(ctx, plan, sha, archivePath)
	if err != nil {
		return res, err
	}
	res.UsedFallback = usedFallback

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return res, err
	}
	binaries, err := ExtractBinaries(archivePath, extractDir)
	if err != nil {
		return res, fmt.Errorf("extract archive: %w", err)
	}

	cliSrc, hasCLI := binaries["shelltime"]
	if plan.CLIDest != "" {
		if !hasCLI {
			return res, fmt.Errorf("archive %s did not contain a shelltime binary", plan.ArchiveName)
		}
		// Smoke-test before touching anything on disk: a corrupt archive, a
		// wrong-arch build, or a macOS signature failure all surface here.
		if err := smokeTestBinary(ctx, cliSrc, plan.Tag); err != nil {
			return res, fmt.Errorf("downloaded shelltime binary failed its smoke test: %w", err)
		}
		if err := ReplaceBinaryWithBackupSuffix(cliSrc, plan.CLIDest, plan.BackupSuffix); err != nil {
			return res, fmt.Errorf("replace shelltime binary: %w", err)
		}
		// Verify what actually landed; roll back if the installed copy is broken.
		if err := smokeTestBinary(ctx, plan.CLIDest, plan.Tag); err != nil {
			if rbErr := RestoreBinaryBackup(plan.CLIDest, plan.BackupSuffix); rbErr != nil {
				return res, fmt.Errorf(
					"installed shelltime binary is broken (%v) AND rollback failed (%v)", err, rbErr)
			}
			return res, fmt.Errorf("installed shelltime binary failed verification, rolled back: %w", err)
		}
		res.ReplacedCLI = true
	}

	daemonSrc, hasDaemon := binaries["shelltime-daemon"]
	if hasDaemon && plan.DaemonDest != "" {
		if err := ReplaceBinaryWithBackupSuffix(daemonSrc, plan.DaemonDest, plan.BackupSuffix); err != nil {
			return res, fmt.Errorf("replace shelltime-daemon binary: %w", err)
		}
		// `daemon install` restores a "<daemon>.bak" believing it is newer.
		// Clear any stale one so it cannot undo this swap.
		_ = os.Remove(plan.DaemonDest + BackupSuffixLegacy)
		res.ReplacedDaemon = true
	}

	if hasDaemon && plan.DaemonStagePath != "" {
		if err := os.MkdirAll(filepath.Dir(plan.DaemonStagePath), 0o755); err != nil {
			return res, err
		}
		_ = os.Remove(plan.DaemonStagePath)
		if err := copyFile(daemonSrc, plan.DaemonStagePath); err != nil {
			return res, fmt.Errorf("stage shelltime-daemon binary: %w", err)
		}
		if err := os.Chmod(plan.DaemonStagePath, 0o755); err != nil {
			return res, err
		}
		res.StagedDaemon = true
	}

	return res, nil
}

// resolveExpectedChecksum prefers the server-provided sha, cross-checking it
// against checksums.txt when both are available.
func resolveExpectedChecksum(ctx context.Context, plan UpdatePlan) (string, error) {
	manifestSha, found, err := FetchChecksumFrom(ctx, plan.Source, plan.Tag, plan.ArchiveName)
	if err != nil {
		slog.Debug("could not fetch checksums.txt", slog.Any("err", err))
	}

	switch {
	case plan.ExpectedSha != "" && found:
		if !strings.EqualFold(plan.ExpectedSha, manifestSha) {
			// The two should agree; a mismatch means something upstream is
			// inconsistent and we should not install anything.
			return "", fmt.Errorf(
				"checksum mismatch between API (%s) and checksums.txt (%s) for %s",
				plan.ExpectedSha, manifestSha, plan.ArchiveName)
		}
		return strings.ToLower(plan.ExpectedSha), nil
	case plan.ExpectedSha != "":
		return strings.ToLower(plan.ExpectedSha), nil
	case found:
		return manifestSha, nil
	default:
		return "", nil
	}
}

// downloadWithFallback tries the configured source, then GitHub directly. The
// fallback matters in the opposite direction from the proxy's purpose: a user
// who CAN reach GitHub should not be blocked by a broken proxy.
func downloadWithFallback(ctx context.Context, plan UpdatePlan, sha, archivePath string) (bool, error) {
	primary := plan.Source.DownloadURL(plan.Tag, plan.ArchiveName)
	err := DownloadAndVerify(ctx, primary, sha, archivePath)
	if err == nil {
		return false, nil
	}
	if !plan.Source.IsProxy() {
		return false, fmt.Errorf("download release: %w", err)
	}

	slog.Warn("release proxy download failed, falling back to GitHub",
		slog.String("url", primary), slog.Any("err", err))

	fallback := plan.Source.Direct().DownloadURL(plan.Tag, plan.ArchiveName)
	if fbErr := DownloadAndVerify(ctx, fallback, sha, archivePath); fbErr != nil {
		return false, fmt.Errorf("download release (proxy: %v; github: %w)", err, fbErr)
	}
	return true, nil
}

// smokeTestBinary runs `<binary> --version` and requires it to exit 0 and report
// the expected tag. Never install a binary that cannot describe itself.
func smokeTestBinary(ctx context.Context, path, expectTag string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() < minBinarySize {
		return fmt.Errorf("binary %s is only %d bytes", path, info.Size())
	}

	ctx, cancel := context.WithTimeout(ctx, smokeTestTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s --version failed: %w (%s)", path, err, strings.TrimSpace(string(out)))
	}
	if expectTag == "" {
		return nil
	}
	want := NormalizeVersion(expectTag)
	if !strings.Contains(string(out), want) {
		return fmt.Errorf("%s --version reported %q, expected it to contain %q",
			path, strings.TrimSpace(string(out)), want)
	}
	return nil
}

// SmokeTestDaemonBinary verifies a staged daemon binary before it is activated.
// The daemon handles -v before any service initialization, so this is fast and
// has no side effects.
func SmokeTestDaemonBinary(ctx context.Context, path, expectTag string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() < minBinarySize {
		return fmt.Errorf("daemon binary %s is only %d bytes", path, info.Size())
	}

	ctx, cancel := context.WithTimeout(ctx, smokeTestTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "-v").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s -v failed: %w (%s)", path, err, strings.TrimSpace(string(out)))
	}
	if expectTag == "" {
		return nil
	}
	want := NormalizeVersion(expectTag)
	if !strings.Contains(string(out), want) {
		return fmt.Errorf("%s -v reported %q, expected it to contain %q",
			path, strings.TrimSpace(string(out)), want)
	}
	return nil
}
