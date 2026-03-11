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

func (z *ZshApp) GetIncludeDirectives() []IncludeDirective {
	return []IncludeDirective{
		{
			OriginalPath:  "~/.zshrc",
			ShelltimePath: "~/.zshrc.shelltime",
			IncludeLine:   "[[ -f ~/.zshrc.shelltime ]] && source ~/.zshrc.shelltime",
			CheckString:   ".zshrc.shelltime",
		},
		{
			OriginalPath:  "~/.zshenv",
			ShelltimePath: "~/.zshenv.shelltime",
			IncludeLine:   "[[ -f ~/.zshenv.shelltime ]] && source ~/.zshenv.shelltime",
			CheckString:   ".zshenv.shelltime",
		},
		{
			OriginalPath:  "~/.zprofile",
			ShelltimePath: "~/.zprofile.shelltime",
			IncludeLine:   "[[ -f ~/.zprofile.shelltime ]] && source ~/.zprofile.shelltime",
			CheckString:   ".zprofile.shelltime",
		},
	}
}

func (z *ZshApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return z.CollectWithIncludeSupport(ctx, z.Name(), z.GetConfigPaths(), &skipIgnored, z.GetIncludeDirectives())
}

func (z *ZshApp) Save(ctx context.Context, files map[string]string, isDryRun bool) error {
	return z.SaveWithIncludeSupport(ctx, files, isDryRun, z.GetIncludeDirectives())
}
