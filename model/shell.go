// service/hook_service.go
package model

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ShellHookService interface {
	Match(shellName string) bool
	// Install sources the hook from the shell's config file, creating that file when
	// it is missing. It changes nothing when the hook is already there.
	Install() error
	Check() error
	Uninstall() error
	ShellName() string
	// ConfigPath is the config file the hook is sourced from.
	ConfigPath() string
}

// ShellHookServices returns the hook service of every supported shell.
func ShellHookServices() []ShellHookService {
	return []ShellHookService{NewZshHookService(), NewFishHookService(), NewBashHookService()}
}

// HookInstallStatus says what InstallShellHooks did for one shell.
type HookInstallStatus int

const (
	// HookSkipped means the shell isn't used here: it has no config file and isn't the
	// login shell.
	HookSkipped HookInstallStatus = iota
	HookInstalled
	HookAlreadyInstalled
	HookFailed
)

// HookInstallResult is what InstallShellHooks did for one shell.
type HookInstallResult struct {
	Shell      string
	ConfigPath string
	Status     HookInstallStatus
	Err        error
}

// InstallShellHooks installs the hook of every shell the user has: one whose config file
// exists, or the login shell (loginShell is $SHELL), whose config file is created when
// missing. The others are skipped, not failed: a machine with only bash has no ~/.zshrc,
// and that is nothing to report.
func InstallShellHooks(services []ShellHookService, loginShell string) []HookInstallResult {
	results := make([]HookInstallResult, 0, len(services))
	for _, svc := range services {
		r := HookInstallResult{Shell: svc.ShellName(), ConfigPath: svc.ConfigPath()}
		if _, err := os.Stat(r.ConfigPath); os.IsNotExist(err) && !isLoginShell(svc, loginShell) {
			r.Status = HookSkipped
			results = append(results, r)
			continue
		}

		alreadyInstalled := svc.Check() == nil
		switch err := svc.Install(); {
		case err != nil:
			r.Status, r.Err = HookFailed, err
		case alreadyInstalled:
			r.Status = HookAlreadyInstalled
		default:
			r.Status = HookInstalled
		}
		results = append(results, r)
	}
	return results
}

// isLoginShell reports whether loginShell, a path like $SHELL, is svc's shell.
func isLoginShell(svc ShellHookService, loginShell string) bool {
	if loginShell == "" {
		return false
	}
	return svc.Match(filepath.Base(loginShell))
}

// ensureHookFile writes the embedded hook script to disk.
// This allows Homebrew-installed users to get hook files without the curl installer.
// An existing file is replaced when its contents differ so `hooks install` picks
// up hook bugfixes. Identical contents are left untouched.
func ensureHookFile(hookFilePath string, embeddedContent []byte) error {
	existing, err := os.ReadFile(hookFilePath)
	if err == nil && bytes.Equal(existing, embeddedContent) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read hook file: %w", err)
	}
	hookDir := filepath.Dir(hookFilePath)
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}
	if err := os.WriteFile(hookFilePath, embeddedContent, 0644); err != nil {
		return fmt.Errorf("failed to write hook file: %w", err)
	}
	return nil
}

// Common utilities for hook services
type BaseHookService struct{}

// ensureConfigFile creates an empty shell config file, and its directory, when there is
// none. Hooks are only installed for shells the user has (see InstallShellHooks), so a
// missing file is one their shell would read once it exists.
func (b *BaseHookService) ensureConfigFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create the directory of %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	return f.Close()
}

func (b *BaseHookService) backupFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	timestamp := time.Now().Format("20060102150405")
	backupPath := fmt.Sprintf("%s.bak.%s", path, timestamp)

	srcFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// Add this new function to BaseHookService
func (b *BaseHookService) removeHookLines(filePath string, hookLines []string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var newLines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		shouldInclude := true
		for _, hookLine := range hookLines {
			if strings.Contains(line, hookLine) {
				shouldInclude = false
				break
			}
		}
		if shouldInclude {
			newLines = append(newLines, line)
		}
	}

	// Write the filtered content back
	if err := os.WriteFile(filePath, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to write updated file: %w", err)
	}

	return nil
}

func (b *BaseHookService) addHookLines(filePath string, hookLines []string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Add hook lines
	lines = append(lines, hookLines...)

	// Write the updated content back
	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to write updated file: %w", err)
	}

	return nil
}
