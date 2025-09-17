package model

import (
	"regexp"

	"github.com/sirupsen/logrus"
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
			logrus.Warnf("Invalid exclude pattern '%s': %v", pattern, err)
			continue
		}

		if re.MatchString(command) {
			logrus.Tracef("Command '%s' matches exclude pattern '%s'", command, pattern)
			return true
		}
	}

	return false
}