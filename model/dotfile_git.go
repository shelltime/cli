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

func (g *GitApp) GetIncludeDirectives() []IncludeDirective {
	return []IncludeDirective{
		{
			OriginalPath:  "~/.gitconfig",
			ShelltimePath: "~/.gitconfig.shelltime",
			IncludeLine:   "[include]\n    path = ~/.gitconfig.shelltime",
			CheckString:   ".gitconfig.shelltime",
		},
		{
			OriginalPath:  "~/.config/git/config",
			ShelltimePath: "~/.config/git/config.shelltime",
			IncludeLine:   "[include]\n    path = ~/.config/git/config.shelltime",
			CheckString:   "git/config.shelltime",
		},
	}
}

func (g *GitApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return g.CollectWithIncludeSupport(ctx, g.Name(), g.GetConfigPaths(), &skipIgnored, g.GetIncludeDirectives())
}

func (g *GitApp) Save(ctx context.Context, files map[string]string, isDryRun bool) error {
	return g.SaveWithIncludeSupport(ctx, files, isDryRun, g.GetIncludeDirectives())
}
