package model

import "context"

// ClaudeApp handles Claude Code configuration files
type ClaudeApp struct {
	BaseApp
}

func NewClaudeApp() DotfileApp {
	return &ClaudeApp{}
}

func (c *ClaudeApp) Name() string {
	return "claude"
}

func (c *ClaudeApp) GetConfigPaths() []string {
	return []string{
		"~/.claude/settings.json",
		"~/.config/claude/settings.json",
		"~/.claude/config.json",
		"~/.config/claude/config.json",
		".claude/settings.json",
		"CLAUDE.md",
	}
}

func (c *ClaudeApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	return c.CollectFromPaths(ctx, c.Name(), c.GetConfigPaths())
}
