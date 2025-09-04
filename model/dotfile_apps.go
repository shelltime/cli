package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/sirupsen/logrus"
)

// DotfileApp interface defines methods for handling app-specific dotfiles
type DotfileApp interface {
	Name() string
	GetConfigPaths() []string
	CollectDotfiles(ctx context.Context) ([]DotfileItem, error)
	IsEqual(ctx context.Context, files map[string]string) (map[string]bool, error)
	Backup(ctx context.Context, paths []string) error
	Save(ctx context.Context, files map[string]string) error
}

// BaseApp provides common functionality for dotfile apps
type BaseApp struct {
	name string
}

func (b *BaseApp) expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(homeDir, path[1:]), nil
	}
	return filepath.Abs(path)
}

func (b *BaseApp) readFileContent(path string) (string, *time.Time, error) {
	expandedPath, err := b.expandPath(path)
	if err != nil {
		return "", nil, err
	}

	fileInfo, err := os.Stat(expandedPath)
	if err != nil {
		return "", nil, err
	}

	content, err := os.ReadFile(expandedPath)
	if err != nil {
		return "", nil, err
	}

	modTime := fileInfo.ModTime()
	return string(content), &modTime, nil
}

func (b *BaseApp) CollectFromPaths(_ context.Context, appName string, paths []string) ([]DotfileItem, error) {
	hostname, _ := os.Hostname()
	var dotfiles []DotfileItem

	for _, path := range paths {
		expandedPath, err := b.expandPath(path)
		if err != nil {
			logrus.Debugf("Failed to expand path %s: %v", path, err)
			continue
		}

		// Check if it's a directory or file
		fileInfo, err := os.Stat(expandedPath)
		if err != nil {
			logrus.Debugf("Path not found: %s", expandedPath)
			continue
		}

		if fileInfo.IsDir() {
			// For directories, collect specific files
			files, err := b.collectFromDirectory(expandedPath)
			if err != nil {
				logrus.Debugf("Failed to collect from directory %s: %v", expandedPath, err)
				continue
			}

			for _, file := range files {
				content, modTime, err := b.readFileContent(file)
				if err != nil {
					logrus.Debugf("Failed to read file %s: %v", file, err)
					continue
				}

				dotfiles = append(dotfiles, DotfileItem{
					App:            appName,
					Path:           file,
					Content:        content,
					FileModifiedAt: modTime,
					FileType:       "file",
					Hostname:       hostname,
				})
			}
		} else {
			// Single file
			content, modTime, err := b.readFileContent(expandedPath)
			if err != nil {
				logrus.Debugf("Failed to read file %s: %v", expandedPath, err)
				continue
			}

			dotfiles = append(dotfiles, DotfileItem{
				App:            appName,
				Path:           expandedPath,
				Content:        content,
				FileModifiedAt: modTime,
				FileType:       "file",
				Hostname:       hostname,
			})
		}
	}

	return dotfiles, nil
}

func (b *BaseApp) collectFromDirectory(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}
		if !info.IsDir() && !strings.HasPrefix(info.Name(), ".") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// IsEqual checks if the provided files match the local files by comparing SHA256 hashes
func (b *BaseApp) IsEqual(_ context.Context, files map[string]string) (map[string]bool, error) {
	result := make(map[string]bool)

	for path, remoteContent := range files {
		expandedPath, err := b.expandPath(path)
		if err != nil {
			logrus.Debugf("Failed to expand path %s: %v", path, err)
			result[path] = false
			continue
		}

		// Read local file content
		localContent, err := os.ReadFile(expandedPath)
		if err != nil {
			if os.IsNotExist(err) {
				// File doesn't exist locally, so it's not equal
				result[path] = false
			} else {
				logrus.Debugf("Failed to read file %s: %v", expandedPath, err)
				result[path] = false
			}
			continue
		}

		// Calculate SHA256 hashes
		localHash := sha256.Sum256(localContent)
		remoteHash := sha256.Sum256([]byte(remoteContent))

		// Compare hashes
		result[path] = fmt.Sprintf("%x", localHash) == fmt.Sprintf("%x", remoteHash)
	}

	return result, nil
}

// Backup creates backups of files that don't match the provided content
func (b *BaseApp) Backup(ctx context.Context, paths []string) error {
	for _, path := range paths {
		expandedPath, err := b.expandPath(path)
		if err != nil {
			logrus.Warnf("Failed to expand path %s: %v", path, err)
			continue
		}

		// Check if file exists
		if _, err := os.Stat(expandedPath); err != nil {
			if !os.IsNotExist(err) {
				logrus.Warnf("Failed to stat file %s: %v", expandedPath, err)
			}
			continue // Skip if file doesn't exist
		}

		// Create backup
		backupPath := expandedPath + ".backup." + time.Now().Format("20060102-150405")
		existingContent, err := os.ReadFile(expandedPath)
		if err != nil {
			logrus.Warnf("Failed to read existing file for backup: %v", err)
			continue
		}

		if err := os.WriteFile(backupPath, existingContent, 0644); err != nil {
			logrus.Warnf("Failed to create backup at %s: %v", backupPath, err)
		} else {
			logrus.Infof("Created backup at %s", backupPath)
		}
	}

	return nil
}

// Save writes new content for files, using diff to check for actual differences
func (b *BaseApp) Save(ctx context.Context, files map[string]string) error {
	dmp := diffmatchpatch.New()

	for path, newContent := range files {
		expandedPath, err := b.expandPath(path)
		if err != nil {
			logrus.Warnf("Failed to expand path %s: %v", path, err)
			continue
		}

		// Read existing content if file exists
		var existingContent string
		if existingBytes, err := os.ReadFile(expandedPath); err == nil {
			existingContent = string(existingBytes)
		} else if !os.IsNotExist(err) {
			logrus.Warnf("Failed to read existing file %s: %v", expandedPath, err)
			continue
		}

		// Check for differences using go-diff
		diffs := dmp.DiffMain(existingContent, newContent, false)
		if len(diffs) == 1 && diffs[0].Type == diffmatchpatch.DiffEqual {
			// No differences found, skip saving
			logrus.Debugf("Skipping %s - content is identical", path)
			continue
		}

		// Ensure directory exists
		dir := filepath.Dir(expandedPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			logrus.Warnf("Failed to create directory %s: %v", dir, err)
			continue
		}

		// Write new content
		if err := os.WriteFile(expandedPath, []byte(newContent), 0644); err != nil {
			logrus.Warnf("Failed to save file %s: %v", expandedPath, err)
		} else {
			logrus.Infof("Saved new content to %s", expandedPath)
		}
	}

	return nil
}
