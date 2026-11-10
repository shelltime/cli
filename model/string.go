package model

import (
	"regexp"
	"strings"
	"sync"
)

// The masking patterns are compiled on first use and kept: BuildTrackingData masks
// every command of a batch, and the daemon builds a batch for each command it
// receives. They are not package-level MustCompile calls, because that would run in
// every `shelltime track` process, most of which never mask anything.
var (
	// A typical JWT token has 3 parts separated by dots and includes base64 characters
	jwtTokenPattern = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`ey[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`)
	})
	longAsteriskRunPattern = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\*{4,}`)
	})
)

// MaskSensitiveTokens masks sensitive tokens (like JWT tokens) in shell commands.
func MaskSensitiveTokens(command string) string {
	// Replace all matches with a masked value
	maskedCommand := jwtTokenPattern().ReplaceAllStringFunc(command, func(token string) string {
		return maskToken(token)
	})

	// If there are more than 3 consecutive asterisks, reduce them to 3
	maskedCommand = longAsteriskRunPattern().ReplaceAllString(maskedCommand, "***")

	return maskedCommand
}

// maskToken masks all but the first and last 4 characters of a token.
func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}
