package model

import "context"

// KittyApp handles Kitty terminal configuration files
type KittyApp struct {
	BaseApp
}

func NewKittyApp() DotfileApp {
	return &KittyApp{}
}

func (k *KittyApp) Name() string {
	return "kitty"
}

func (k *KittyApp) GetConfigPaths() []string {
	return []string{
		"~/.config/kitty/",
	}
}

func (k *KittyApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	return k.CollectFromPaths(ctx, k.Name(), k.GetConfigPaths())
}
