package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestShouldExcludeCommand(t *testing.T) {
	tests := []struct {
		name            string
		command         string
		excludePatterns []string
		want            bool
	}{
		{
			name:            "No patterns - should not exclude",
			command:         "ls -la",
			excludePatterns: []string{},
			want:            false,
		},
		{
			name:            "Empty pattern - should not exclude",
			command:         "ls -la",
			excludePatterns: []string{""},
			want:            false,
		},
		{
			name:            "Simple match - should exclude",
			command:         "git secret",
			excludePatterns: []string{"secret"},
			want:            true,
		},
		{
			name:            "Regex pattern match - should exclude",
			command:         "cat /etc/passwd",
			excludePatterns: []string{"^cat.*passwd$"},
			want:            true,
		},
		{
			name:            "Multiple patterns, one matches - should exclude",
			command:         "ssh root@server",
			excludePatterns: []string{"^ls", "^ssh.*root", "^cat"},
			want:            true,
		},
		{
			name:            "Multiple patterns, none match - should not exclude",
			command:         "docker ps",
			excludePatterns: []string{"^ls", "^ssh", "^cat"},
			want:            false,
		},
		{
			name:            "Pattern for sensitive commands - should exclude",
			command:         "export AWS_SECRET_ACCESS_KEY=xyz",
			excludePatterns: []string{"AWS_SECRET", "PASSWORD", "TOKEN"},
			want:            true,
		},
		{
			name:            "Invalid regex pattern - should skip and not exclude",
			command:         "ls -la",
			excludePatterns: []string{"[invalid"},
			want:            false,
		},
		{
			name:            "Case sensitive match",
			command:         "echo SECRET",
			excludePatterns: []string{"secret"},
			want:            false,
		},
		{
			name:            "Case insensitive regex",
			command:         "echo SECRET",
			excludePatterns: []string{"(?i)secret"},
			want:            true,
		},
		{
			name:            "Exclude history commands",
			command:         "history | grep password",
			excludePatterns: []string{"^history"},
			want:            true,
		},
		{
			name:            "Exclude gpg commands",
			command:         "gpg --decrypt file.gpg",
			excludePatterns: []string{"^gpg.*--decrypt"},
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldExcludeCommand(tt.command, tt.excludePatterns)
			if got != tt.want {
				t.Errorf("ShouldExcludeCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Patterns are compiled once and kept, including the fact that one doesn't compile.
// Later calls must give the same answers as the first.
func TestShouldExcludeCommand_RepeatedCalls(t *testing.T) {
	patterns := []string{"[invalid", "", "^ssh .*@prod"}
	for range 3 {
		if !ShouldExcludeCommand("ssh deploy@prod-1", patterns) {
			t.Fatal("a pattern after an invalid one must still exclude")
		}
		if ShouldExcludeCommand("ssh deploy@staging", patterns) {
			t.Fatal("a command no pattern matches must not be excluded")
		}
		if ShouldExcludeCommand("ls [invalid", []string{"[invalid"}) {
			t.Fatal("an invalid pattern excludes nothing")
		}
	}
}

// The daemon matches commands from several goroutines (socket handlers build
// batches while the processor filters events), so the pattern caches must be safe
// to fill concurrently. Run with -race.
func TestExcludeAndMaskPatterns_ConcurrentUse(t *testing.T) {
	const jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			pattern := fmt.Sprintf("^concurrent-%d ", i%4)
			for range 50 {
				if !ShouldExcludeCommand(fmt.Sprintf("concurrent-%d run", i%4), []string{"[broken", pattern}) {
					t.Error("command must be excluded")
				}
				if got := MaskSensitiveTokens("curl -H 'Authorization: Bearer " + jwt + "' x"); strings.Contains(got, jwt) {
					t.Error("token must be masked")
				}
			}
		})
	}
	wg.Wait()
}
