// service/hook_service.go
package model

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type ShellHookService interface {
	Match(shellName string) bool
	Install() error
	Check() error
	Uninstall() error
	ShellName() string
}

// Common utilities for hook services
type BaseHookService struct{}

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
