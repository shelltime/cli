package model

import (
	"log/slog"
	"regexp"
)

// ShouldExcludeCommand checks if a command matches any of the exclude patterns
func ShouldExcludeCommand(command string, excludePatterns []string) bool {
	if len(excludePatterns) == 0 {
		return false
	}

	for _, pattern := range excludePatterns {
		if pattern == "" {
			continue
		}

		re, err := regexp.Compile(pattern)
		if err != nil {
			slog.Warn("Invalid exclude pattern", slog.String("pattern", pattern), slog.Any("err", err))
			continue
		}

		if re.MatchString(command) {
			slog.Debug("Command matches exclude pattern", slog.String("command", command), slog.String("pattern", pattern))
			return true
		}
	}

	return false
}