package model

import "context"

// GhosttyApp handles Ghostty terminal configuration files
type GhosttyApp struct {
	BaseApp
}

func NewGhosttyApp() DotfileApp {
	return &GhosttyApp{}
}

func (g *GhosttyApp) Name() string {
	return "ghostty"
}

func (g *GhosttyApp) GetConfigPaths() []string {
	return []string{
		"~/.config/ghostty/config",
		"~/.config/ghostty",
	}
}

func (g *GhosttyApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return g.CollectFromPaths(ctx, g.Name(), g.GetConfigPaths(), &skipIgnored)
}