package model

import "context"

// ZshApp handles Zsh shell configuration files
type ZshApp struct {
	BaseApp
}

func NewZshApp() DotfileApp {
	return &ZshApp{}
}

func (z *ZshApp) Name() string {
	return "zsh"
}

func (z *ZshApp) GetConfigPaths() []string {
	return []string{
		"~/.zshrc",
		"~/.zshenv",
		"~/.zprofile",
		"~/.config/zsh",
	}
}

func (z *ZshApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return z.CollectFromPaths(ctx, z.Name(), z.GetConfigPaths(), &skipIgnored)
}