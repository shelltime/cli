package model

import (
	"log/slog"
	"regexp"
	"sync"
)

// compiledExcludePatterns holds each exclude pattern compiled, or a nil *regexp.Regexp for
// one that doesn't compile. The patterns come from the config, so there are only a
// few, while they are matched against every command: twice per command in the
// daemon, and once per unsent command each time a batch is built.
var compiledExcludePatterns sync.Map // pattern string -> *regexp.Regexp

// compiledExcludePattern returns pattern compiled, or nil when it is not a valid
// regular expression, which is logged the first time it is seen.
func compiledExcludePattern(pattern string) *regexp.Regexp {
	if cached, ok := compiledExcludePatterns.Load(pattern); ok {
		return cached.(*regexp.Regexp)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		slog.Warn("Invalid exclude pattern", slog.String("pattern", pattern), slog.Any("err", err))
		re = nil
	}
	compiledExcludePatterns.Store(pattern, re)
	return re
}

// ShouldExcludeCommand checks if a command matches any of the exclude patterns
func ShouldExcludeCommand(command string, excludePatterns []string) bool {
	if len(excludePatterns) == 0 {
		return false
	}

	for _, pattern := range excludePatterns {
		if pattern == "" {
			continue
		}

		re := compiledExcludePattern(pattern)
		if re == nil {
			continue
		}

		if re.MatchString(command) {
			slog.Debug("Command matches exclude pattern", slog.String("command", command), slog.String("pattern", pattern))
			return true
		}
	}

	return false
}
