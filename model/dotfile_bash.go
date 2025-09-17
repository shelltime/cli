package model

import "context"

// BashApp handles Bash shell configuration files
type BashApp struct {
	BaseApp
}

func NewBashApp() DotfileApp {
	return &BashApp{}
}

func (b *BashApp) Name() string {
	return "bash"
}

func (b *BashApp) GetConfigPaths() []string {
	return []string{
		"~/.bashrc",
		"~/.bash_profile",
		"~/.bash_aliases",
		"~/.bash_logout",
	}
}

func (b *BashApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return b.CollectFromPaths(ctx, b.Name(), b.GetConfigPaths(), &skipIgnored)
}