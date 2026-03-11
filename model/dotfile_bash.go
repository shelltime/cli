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

func (b *BashApp) GetIncludeDirectives() []IncludeDirective {
	return []IncludeDirective{
		{
			OriginalPath:  "~/.bashrc",
			ShelltimePath: "~/.bashrc.shelltime",
			IncludeLine:   "[[ -f ~/.bashrc.shelltime ]] && source ~/.bashrc.shelltime",
			CheckString:   ".bashrc.shelltime",
		},
		{
			OriginalPath:  "~/.bash_profile",
			ShelltimePath: "~/.bash_profile.shelltime",
			IncludeLine:   "[[ -f ~/.bash_profile.shelltime ]] && source ~/.bash_profile.shelltime",
			CheckString:   ".bash_profile.shelltime",
		},
		{
			OriginalPath:  "~/.bash_aliases",
			ShelltimePath: "~/.bash_aliases.shelltime",
			IncludeLine:   "[[ -f ~/.bash_aliases.shelltime ]] && source ~/.bash_aliases.shelltime",
			CheckString:   ".bash_aliases.shelltime",
		},
		{
			OriginalPath:  "~/.bash_logout",
			ShelltimePath: "~/.bash_logout.shelltime",
			IncludeLine:   "[[ -f ~/.bash_logout.shelltime ]] && source ~/.bash_logout.shelltime",
			CheckString:   ".bash_logout.shelltime",
		},
	}
}

func (b *BashApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return b.CollectWithIncludeSupport(ctx, b.Name(), b.GetConfigPaths(), &skipIgnored, b.GetIncludeDirectives())
}

func (b *BashApp) Save(ctx context.Context, files map[string]string, isDryRun bool) error {
	return b.SaveWithIncludeSupport(ctx, files, isDryRun, b.GetIncludeDirectives())
}
