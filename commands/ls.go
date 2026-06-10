// cli/commands/ls.go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gookit/color"
	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
)

var LsCommand *cli.Command = &cli.Command{
	Name:  "ls",
	Usage: "list locally saved commands",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "format",
			Aliases: []string{"f"},
			Value:   "table",
			Usage:   "output format (table/json)",
		},
	},
	Action: commandList,
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		color.Red.Println(err.Error())
		return nil
	},
}

func commandList(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "ls", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	format := c.String("format")
	if format != "table" && format != "json" {
		return fmt.Errorf("unsupported format: %s. Use 'table' or 'json'", format)
	}

	// TODO: add un-sync data list here
	if format == "table" {
		color.Yellow.Println("⚠️ Note: Unsaved commands are not included in this list")
	}

	if format == "table" {
		color.Yellow.Println("⚠️ Note: Local data will be cleaned periodically for performance and disk efficiency. To view all of your commands, please run 'shelltime web'")
	}

	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		return err
	}

	// In bolt mode the daemon owns the (exclusively locked) DB, so query it over
	// the socket. Otherwise read the txt file store directly.
	var commands []model.ListedCommand
	useBolt := config.Storage != nil && config.Storage.Engine == model.StorageEngineBolt
	if useBolt && daemon.IsSocketReady(ctx, config.SocketPath) {
		resp, err := daemon.RequestListCommands(config.SocketPath, 2*time.Second)
		if err != nil {
			return err
		}
		commands = resp.Commands
	} else {
		commands, err = model.BuildListedCommands(ctx, model.NewFileStore())
		if err != nil {
			return err
		}
	}

	// Output based on format
	if format == "json" {
		return outputJSON(commands)
	}
	return outputTable(commands)
}

func outputJSON(commands interface{}) error {
	jsonData, err := json.MarshalIndent(commands, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

func outputTable(commands []model.ListedCommand) error {
	w := tablewriter.NewWriter(os.Stdout)
	w.Header([]string{"COMMAND", "SHELL", "START TIME", "END TIME", "DURATION(ms)", "STATUS", "USER", "HOST"})

	for _, cmd := range commands {
		duration := cmd.EndTime.Sub(cmd.StartTime).Milliseconds()
		w.Append([]string{
			cmd.Command,
			cmd.Shell,
			cmd.StartTime.Format(time.RFC3339),
			cmd.EndTime.Format(time.RFC3339),
			strconv.Itoa(int(duration)),
			strconv.Itoa(cmd.Result),
			cmd.Username,
			cmd.Hostname,
		})
	}

	w.Render()
	return nil
}
