package model

import (
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