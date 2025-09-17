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

func (f *FishApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return f.CollectFromPaths(ctx, f.Name(), f.GetConfigPaths(), &skipIgnored)
}