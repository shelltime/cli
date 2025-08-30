package model

import "context"

// GitApp handles Git configuration files
type GitApp struct {
	BaseApp
}

func NewGitApp() DotfileApp {
	return &GitApp{}
}

func (g *GitApp) Name() string {
	return "git"
}

func (g *GitApp) GetConfigPaths() []string {
	return []string{
		"~/.gitconfig",
		"~/.gitignore_global",
		"~/.config/git/config",
		"~/.config/git/ignore",
	}
}

func (g *GitApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	return g.CollectFromPaths(ctx, g.Name(), g.GetConfigPaths())
}