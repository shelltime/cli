package model

import "context"

// FishApp handles Fish shell configuration files
type FishApp struct {
	BaseApp
}

func NewFishApp() DotfileApp {
	return &FishApp{}
}

func (f *FishApp) Name() string {
	return "fish"
}

func (f *FishApp) GetConfigPaths() []string {
	return []string{
		"~/.config/fish/config.fish",
		"~/.config/fish/functions",
		"~/.config/fish/conf.d",
	}
}

func (f *FishApp) GetIncludeDirectives() []IncludeDirective {
	return []IncludeDirective{
		{
			OriginalPath:  "~/.config/fish/config.fish",
			ShelltimePath: "~/.config/fish/config.fish.shelltime",
			IncludeLine:   "test -f ~/.config/fish/config.fish.shelltime; and source ~/.config/fish/config.fish.shelltime",
			CheckString:   "config.fish.shelltime",
		},
	}
}

func (f *FishApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return f.CollectWithIncludeSupport(ctx, f.Name(), f.GetConfigPaths(), &skipIgnored, f.GetIncludeDirectives())
}

func (f *FishApp) Save(ctx context.Context, files map[string]string, isDryRun bool) error {
	return f.SaveWithIncludeSupport(ctx, files, isDryRun, f.GetIncludeDirectives())
}
