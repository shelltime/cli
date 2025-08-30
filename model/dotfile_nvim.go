package model

import "context"

// NvimApp handles Neovim configuration files
type NvimApp struct {
	BaseApp
}

func NewNvimApp() DotfileApp {
	return &NvimApp{}
}

func (n *NvimApp) Name() string {
	return "nvim"
}

func (n *NvimApp) GetConfigPaths() []string {
	return []string{
		"~/.config/nvim",
		"~/.vimrc",
	}
}

func (n *NvimApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	return n.CollectFromPaths(ctx, n.Name(), n.GetConfigPaths())
}