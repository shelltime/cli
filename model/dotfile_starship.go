package model

import "context"

// StarshipApp handles Starship configuration files
type StarshipApp struct {
	BaseApp
}

func NewStarshipApp() DotfileApp {
	return &StarshipApp{}
}

func (s *StarshipApp) Name() string {
	return "starship"
}

func (s *StarshipApp) GetConfigPaths() []string {
	return []string{
		"~/.config/starship.toml",
	}
}

func (s *StarshipApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	return s.CollectFromPaths(ctx, s.Name(), s.GetConfigPaths())
}
