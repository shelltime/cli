package model

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// IncludeDirective defines how a config file includes its shelltime-managed version.
// When pushing/pulling dotfiles, the original config file gets an include line added
// at the top that sources the .shelltime version. The actual synced content lives
// in the .shelltime file.
type IncludeDirective struct {
	OriginalPath  string // tilde path to original config, e.g., "~/.gitconfig"
	ShelltimePath string // tilde path to shelltime file, e.g., "~/.gitconfig.shelltime"
	IncludeLine   string // The include line(s) to add at the top of the original file
	CheckString   string // Substring to check if include already exists in file content
}

// ensureIncludeSetup ensures the include directive is properly set up for push.
// - If the original file has no include line and no .shelltime file exists:
//
//	copies content to .shelltime and adds include line to original.
//
// - If the include line is missing: adds it to the original.
// - If .shelltime file is missing but include line exists: extracts content from original.
func (b *BaseApp) ensureIncludeSetup(directive *IncludeDirective) error {
	expandedOriginal, err := b.expandPath(directive.OriginalPath)
	if err != nil {
		return err
	}
	expandedShelltime, err := b.expandPath(directive.ShelltimePath)
	if err != nil {
		return err
	}

	// Check if original file exists
	originalBytes, err := os.ReadFile(expandedOriginal)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Original doesn't exist, nothing to do
		}
		return err
	}

	content := string(originalBytes)
	hasInclude := strings.Contains(content, directive.CheckString)
	_, shelltimeErr := os.Stat(expandedShelltime)
	shelltimeExists := shelltimeErr == nil

	if hasInclude && shelltimeExists {
		return nil // Already set up
	}

	if !hasInclude && !shelltimeExists {
		// First time setup: copy content to .shelltime, add include to original
		if err := writeFileWithDir(expandedShelltime, content); err != nil {
			return err
		}
		newOriginal := directive.IncludeLine + "\n" + content
		return os.WriteFile(expandedOriginal, []byte(newOriginal), 0644)
	}

	if !hasInclude {
		// .shelltime exists but include line missing from original - add it
		newOriginal := directive.IncludeLine + "\n" + content
		return os.WriteFile(expandedOriginal, []byte(newOriginal), 0644)
	}

	// hasInclude && !shelltimeExists
	// Include line exists but .shelltime file was deleted - extract content
	contentWithoutInclude := removeIncludeLines(content, directive)
	return writeFileWithDir(expandedShelltime, contentWithoutInclude)
}

// ensureIncludeLineInFile ensures the include line exists in the original config file.
// Used during pull to set up the include before saving the .shelltime file.
func (b *BaseApp) ensureIncludeLineInFile(directive *IncludeDirective, isDryRun bool) error {
	expandedOriginal, err := b.expandPath(directive.OriginalPath)
	if err != nil {
		return err
	}

	// Read original file (or treat as empty if it doesn't exist)
	var content string
	if data, err := os.ReadFile(expandedOriginal); err == nil {
		content = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}

	// Check if include already exists
	if strings.Contains(content, directive.CheckString) {
		return nil
	}

	if isDryRun {
		slog.Info("[DRY RUN] Would add include line", slog.String("file", expandedOriginal))
		return nil
	}

	// Add include line at top
	var newContent string
	if content == "" {
		newContent = directive.IncludeLine + "\n"
	} else {
		newContent = directive.IncludeLine + "\n" + content
	}

	return writeFileWithDir(expandedOriginal, newContent)
}

// CollectWithIncludeSupport collects dotfiles, handling include directives.
// For paths with include directives, it ensures the include setup and collects from .shelltime files.
// For other paths (directories, non-includable files), it uses standard collection.
func (b *BaseApp) CollectWithIncludeSupport(ctx context.Context, appName string, paths []string, skipIgnored *bool, directives []IncludeDirective) ([]DotfileItem, error) {
	// Build directive lookup by expanded original path
	directiveMap := make(map[string]*IncludeDirective)
	for i, d := range directives {
		expanded, err := b.expandPath(d.OriginalPath)
		if err == nil {
			directiveMap[expanded] = &directives[i]
		}
	}

	var allDotfiles []DotfileItem
	var nonIncludePaths []string

	for _, path := range paths {
		expanded, err := b.expandPath(path)
		if err != nil {
			nonIncludePaths = append(nonIncludePaths, path)
			continue
		}

		directive, found := directiveMap[expanded]
		if !found {
			nonIncludePaths = append(nonIncludePaths, path)
			continue
		}

		// This path has include support
		// Check if original file exists
		if _, err := os.Stat(expanded); err != nil {
			slog.Debug("Original file not found, skipping include setup", slog.String("path", path))
			continue
		}

		// Ensure include setup (adds include line, creates .shelltime file if needed)
		if err := b.ensureIncludeSetup(directive); err != nil {
			slog.Warn("Failed to ensure include setup", slog.String("path", path), slog.Any("err", err))
			continue
		}

		// Collect from .shelltime file instead of original
		items, err := b.CollectFromPaths(ctx, appName, []string{directive.ShelltimePath}, skipIgnored)
		if err != nil {
			slog.Warn("Failed to collect from shelltime file", slog.String("path", directive.ShelltimePath), slog.Any("err", err))
			continue
		}
		allDotfiles = append(allDotfiles, items...)
	}

	// Collect non-include paths normally (directories, non-includable files)
	if len(nonIncludePaths) > 0 {
		items, err := b.CollectFromPaths(ctx, appName, nonIncludePaths, skipIgnored)
		if err != nil {
			return allDotfiles, err
		}
		allDotfiles = append(allDotfiles, items...)
	}

	return allDotfiles, nil
}

// SaveWithIncludeSupport saves files, ensuring include directives for .shelltime files.
// For .shelltime paths that match a known directive, it also ensures the include line
// exists in the original config file.
func (b *BaseApp) SaveWithIncludeSupport(ctx context.Context, files map[string]string, isDryRun bool, directives []IncludeDirective) error {
	// Build directive lookup by shelltime path (both tilde and expanded)
	shelltimeMap := make(map[string]*IncludeDirective)
	for i, d := range directives {
		shelltimeMap[d.ShelltimePath] = &directives[i]
		expanded, err := b.expandPath(d.ShelltimePath)
		if err == nil {
			shelltimeMap[expanded] = &directives[i]
		}
	}

	// For .shelltime files, ensure include line in the original config
	for filePath := range files {
		if directive, found := shelltimeMap[filePath]; found {
			if err := b.ensureIncludeLineInFile(directive, isDryRun); err != nil {
				slog.Warn("Failed to ensure include line", slog.String("path", filePath), slog.Any("err", err))
			}
		}
	}

	// Use base Save for actual file writing
	return b.Save(ctx, files, isDryRun)
}

// writeFileWithDir writes content to a file, creating parent directories if needed.
func writeFileWithDir(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// removeIncludeLines removes the include directive lines from content.
// First tries to match and remove from the top of the file.
// Falls back to removing any lines containing the check string.
func removeIncludeLines(content string, directive *IncludeDirective) string {
	lines := strings.Split(content, "\n")
	includeLines := strings.Split(directive.IncludeLine, "\n")

	// Try to find and remove the include lines from the top
	if len(lines) >= len(includeLines) {
		allMatch := true
		for i, il := range includeLines {
			if strings.TrimSpace(lines[i]) != strings.TrimSpace(il) {
				allMatch = false
				break
			}
		}
		if allMatch {
			remaining := lines[len(includeLines):]
			// Remove leading empty line if present (we add \n after include line)
			if len(remaining) > 0 && strings.TrimSpace(remaining[0]) == "" {
				remaining = remaining[1:]
			}
			return strings.Join(remaining, "\n")
		}
	}

	// Fallback: remove any line containing the check string
	var filtered []string
	for _, line := range lines {
		if !strings.Contains(line, directive.CheckString) {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}
