package model

import "context"

// NpmApp handles npm configuration files
type NpmApp struct {
	BaseApp
}

func NewNpmApp() DotfileApp {
	return &NpmApp{}
}

func (n *NpmApp) Name() string {
	return "npm"
}

func (n *NpmApp) GetConfigPaths() []string {
	return []string{
		"~/.npmrc",
	}
}

func (n *NpmApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	return n.CollectFromPaths(ctx, n.Name(), n.GetConfigPaths())
}
