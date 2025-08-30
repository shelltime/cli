package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// DotfileApp interface defines methods for handling app-specific dotfiles
type DotfileApp interface {
	Name() string
	GetConfigPaths() []string
	CollectDotfiles(ctx context.Context) ([]DotfileItem, error)
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