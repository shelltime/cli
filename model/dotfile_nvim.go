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

func (n *NvimApp) GetIncludeDirectives() []IncludeDirective {
	return []IncludeDirective{
		{
			OriginalPath:  "~/.vimrc",
			ShelltimePath: "~/.vimrc.shelltime",
			IncludeLine:   "if filereadable(expand('~/.vimrc.shelltime')) | source ~/.vimrc.shelltime | endif",
			CheckString:   ".vimrc.shelltime",
		},
	}
}

func (n *NvimApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return n.CollectWithIncludeSupport(ctx, n.Name(), n.GetConfigPaths(), &skipIgnored, n.GetIncludeDirectives())
}

func (n *NvimApp) Save(ctx context.Context, files map[string]string, isDryRun bool) error {
	return n.SaveWithIncludeSupport(ctx, files, isDryRun, n.GetIncludeDirectives())
}
