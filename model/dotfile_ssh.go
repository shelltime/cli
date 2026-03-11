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

func (s *SshApp) GetIncludeDirectives() []IncludeDirective {
	return []IncludeDirective{
		{
			OriginalPath:  "~/.ssh/config",
			ShelltimePath: "~/.ssh/config.shelltime",
			IncludeLine:   "Include ~/.ssh/config.shelltime",
			CheckString:   "config.shelltime",
		},
	}
}

func (s *SshApp) CollectDotfiles(ctx context.Context) ([]DotfileItem, error) {
	skipIgnored := true
	return s.CollectWithIncludeSupport(ctx, s.Name(), s.GetConfigPaths(), &skipIgnored, s.GetIncludeDirectives())
}

func (s *SshApp) Save(ctx context.Context, files map[string]string, isDryRun bool) error {
	return s.SaveWithIncludeSupport(ctx, files, isDryRun, s.GetIncludeDirectives())
}
