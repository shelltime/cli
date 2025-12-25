package model

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// GhosttyApp handles Ghostty terminal configuration files
type GhosttyApp struct {
	BaseApp
}

func NewGhosttyApp() DotfileApp {
	return &GhosttyApp{}
}

func (g *GhosttyApp) Name() string {
	return "ghostty"
}

func (g *GhosttyApp) GetConfigPaths() []string {
	return []string{
		"~/.config/ghostty/config",
	}
}

func (g *GhosttyApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return g.CollectFromPaths(ctx, g.Name(), g.GetConfigPaths(), &skipIgnored)
}

// configLine represents a line in the Ghostty config file
type configLine struct {
	isComment bool
	isBlank   bool
	key       string
	value     string
	raw       string // for comments and blank lines
}

// parseGhosttyConfig parses Ghostty config content into structured lines
func (g *GhosttyApp) parseGhosttyConfig(content string) []configLine {
	var lines []configLine
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			lines = append(lines, configLine{
				isBlank: true,
				raw:     line,
			})
		} else if strings.HasPrefix(trimmed, "#") {
			lines = append(lines, configLine{
				isComment: true,
				raw:       line,
			})
		} else if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := ""
			if len(parts) > 1 {
				value = strings.TrimSpace(parts[1])
			}
			lines = append(lines, configLine{
				key:   key,
				value: value,
				raw:   line,
			})
		} else {
			// Treat as a comment if it doesn't match key=value format
			lines = append(lines, configLine{
				isComment: true,
				raw:       line,
			})
		}
	}

	return lines
}

// mergeGhosttyConfigs merges remote config with local config, local has priority
func (g *GhosttyApp) mergeGhosttyConfigs(localLines, remoteLines []configLine) []configLine {
	// Create a map of local keys for quick lookup
	localKeys := make(map[string]bool)
	for _, line := range localLines {
		if !line.isComment && !line.isBlank && line.key != "" {
			localKeys[line.key] = true
		}
	}

	// Start with local config
	merged := make([]configLine, len(localLines))
	copy(merged, localLines)

	// Add keys from remote that don't exist in local
	for _, remoteLine := range remoteLines {
		if !remoteLine.isComment && !remoteLine.isBlank && remoteLine.key != "" {
			if !localKeys[remoteLine.key] {
				merged = append(merged, remoteLine)
			}
		}
	}

	return merged
}

// formatGhosttyConfig converts config lines back to string
func (g *GhosttyApp) formatGhosttyConfig(lines []configLine) string {
	var result []string
	for _, line := range lines {
		if line.isComment || line.isBlank {
			result = append(result, line.raw)
		} else {
			result = append(result, fmt.Sprintf("%s = %s", line.key, line.value))
		}
	}
	return strings.Join(result, "\n")
}

// Save overrides the base Save method to handle Ghostty's specific config format
func (g *GhosttyApp) Save(ctx context.Context, files map[string]string, isDryRun bool) error {
	for path, remoteContent := range files {
		expandedPath, err := g.expandPath(path)
		if err != nil {
			slog.Warn("Failed to expand path", slog.String("path", path), slog.Any("err", err))
			continue
		}

		// Read existing local content if file exists
		var localContent string
		if existingBytes, err := os.ReadFile(expandedPath); err == nil {
			localContent = string(existingBytes)
		} else if !os.IsNotExist(err) {
			slog.Warn("Failed to read existing file", slog.String("path", expandedPath), slog.Any("err", err))
			continue
		}

		// Parse both configs
		localLines := g.parseGhosttyConfig(localContent)
		remoteLines := g.parseGhosttyConfig(remoteContent)

		// Merge configs (local has priority)
		mergedLines := g.mergeGhosttyConfigs(localLines, remoteLines)
		mergedContent := g.formatGhosttyConfig(mergedLines)

		// Check if there are any differences
		if localContent == mergedContent {
			slog.Info("No changes needed", slog.String("path", expandedPath))
			continue
		}

		if isDryRun {
			// In dry-run mode, show the diff
			fmt.Printf("\n📄 %s:\n", expandedPath)
			fmt.Println("--- Changes to be applied ---")

			// Show added keys (from remote)
			remoteKeys := make(map[string]string)
			for _, line := range remoteLines {
				if !line.isComment && !line.isBlank && line.key != "" {
					remoteKeys[line.key] = line.value
				}
			}

			localKeys := make(map[string]string)
			for _, line := range localLines {
				if !line.isComment && !line.isBlank && line.key != "" {
					localKeys[line.key] = line.value
				}
			}

			// Show new keys from remote
			hasChanges := false
			for key, value := range remoteKeys {
				if _, exists := localKeys[key]; !exists {
					fmt.Printf("+ %s = %s (from remote)\n", key, value)
					hasChanges = true
				}
			}

			if !hasChanges {
				fmt.Println("No new keys from remote")
			}

			fmt.Println("--- End of changes ---")
			continue
		}

		// Ensure directory exists
		dir := filepath.Dir(expandedPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Warn("Failed to create directory", slog.String("dir", dir), slog.Any("err", err))
			continue
		}

		// Write merged content
		if err := os.WriteFile(expandedPath, []byte(mergedContent), 0644); err != nil {
			slog.Warn("Failed to save file", slog.String("path", expandedPath), slog.Any("err", err))
		} else {
			slog.Info("Saved merged config", slog.String("path", expandedPath))
		}
	}

	return nil
}
