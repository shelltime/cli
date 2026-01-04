package commands

import (
	"github.com/urfave/cli/v2"
)

var DotfilesCommand *cli.Command = &cli.Command{
	Name:  "dotfiles",
	Usage: "manage dotfiles configuration",
	Subcommands: []*cli.Command{
		{
			Name:   "push",
			Usage:  "push dotfiles to server",
			Action: pushDotfiles,
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "apps",
					Aliases: []string{"a"},
					Usage:   "specify which apps to push (nvim, fish, git, zsh, bash, ghostty). If empty, pushes all",
				},
			},
		},
		{
			Name:   "pull",
			Usage:  "pull dotfiles from server and save to local config",
			Action: pullDotfiles,
			Flags: []cli.Flag{
				&cli.StringSliceFlag{
					Name:    "apps",
					Aliases: []string{"a"},
					Usage:   "specify which apps to pull (nvim, fish, git, zsh, bash, ghostty). If empty, pulls all",
				},
				&cli.BoolFlag{
					Name:    "dry-run",
					Aliases: []string{"n"},
					Usage:   "show what would be changed without actually writing files",
				},
			},
		},
	},
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		return nil
	},
}
