package commands

import (
	"fmt"

	"github.com/gookit/color"
	"github.com/pkg/browser"
	"github.com/urfave/cli/v2"
)

const appStoreURL = "https://apps.apple.com/us/app/shelltime-xyz/id6757661383"

var IosCommand *cli.Command = &cli.Command{
	Name:  "ios",
	Usage: "iOS app related commands",
	Subcommands: []*cli.Command{
		{
			Name:   "dl",
			Usage:  "open the ShellTime iOS app download page on the App Store",
			Action: commandIosDl,
			OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
				color.Red.Println(err.Error())
				return nil
			},
		},
	},
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		color.Red.Println(err.Error())
		return nil
	},
}

func commandIosDl(c *cli.Context) error {
	color.Green.Printf("ShellTime iOS App Store URL:\n%s\n", appStoreURL)

	err := browser.OpenURL(appStoreURL)
	if err != nil {
		fmt.Printf("Could not open browser automatically. Please visit the URL above to download.\n")
	}

	return nil
}
