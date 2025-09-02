package model

import "context"

// SshApp handles SSH configuration files
type SshApp struct {
	BaseApp
}

func NewSshApp() DotfileApp {
	return &SshApp{}
}

func (s *SshApp) Name() string {
	return "ssh"
}

func (s *SshApp) GetConfigPaths() []string {
	return []string{
		"~/.ssh/config",
	}
}

func (s *SshApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	return s.CollectFromPaths(ctx, s.Name(), s.GetConfigPaths())
}
